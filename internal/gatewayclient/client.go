// Package gatewayclient owns route-scoped TLS policy for gateway traffic and probes.
package gatewayclient

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

var ErrFingerprint = errors.New("gateway certificate fingerprint changed")
var ErrValidity = errors.New("gateway certificate outside validity period")

func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func VerifyPinned(cs tls.ConnectionState, fingerprint string) error {
	if len(cs.PeerCertificates) == 0 || !strings.EqualFold(Fingerprint(cs.PeerCertificates[0]), fingerprint) {
		return ErrFingerprint
	}
	cert := cs.PeerCertificates[0]
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return ErrValidity
	}
	return nil
}

func newTransport() *http.Transport {
	return &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 100, MaxIdleConnsPerHost: 10, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 15 * time.Second, ExpectContinueTimeout: time.Second}
}

var strictTransport = newTransport()
var pinnedCache = struct {
	sync.Mutex
	transports map[string]*http.Transport
}{transports: map[string]*http.Transport{}}

func Transport(route domain.GatewayRoute) *http.Transport {
	if route.TLSFingerprint == "" {
		return strictTransport
	}
	// Never apply an old pin to a different destination, even with corrupt storage.
	target, err := url.Parse(route.Target)
	if err != nil || target.Scheme != "https" || route.TLSTrustTarget != domain.GatewayTrustTarget(route.Target) {
		return strictTransport
	}
	key := route.TLSTrustTarget + "|" + route.TLSFingerprint
	pinnedCache.Lock()
	defer pinnedCache.Unlock()
	if transport := pinnedCache.transports[key]; transport != nil {
		return transport
	}
	transport := newTransport()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error { return VerifyPinned(cs, route.TLSFingerprint) }} // exact leaf pin replaces PKI/hostname verification, never certificate validity checks
	if len(pinnedCache.transports) >= 128 {
		for oldKey, old := range pinnedCache.transports {
			old.CloseIdleConnections()
			delete(pinnedCache.transports, oldKey)
			break
		}
	}
	pinnedCache.transports[key] = transport
	return transport
}

type Certificate struct {
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	Fingerprint string    `json:"fingerprint"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Names       []string  `json:"names"`
}
type ProbeResult struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Target      string       `json:"target"`
	HTTPStatus  int          `json:"http_status,omitempty"`
	Certificate *Certificate `json:"certificate,omitempty"`
	CanTrust    bool         `json:"can_trust"`
}

func inspectTLS(ctx context.Context, target *url.URL) (tls.ConnectionState, error) {
	address := target.Host
	if target.Port() == "" {
		address = net.JoinHostPort(target.Hostname(), "443")
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}} // Certificate inspection only. Application data uses the verified transport.
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return tls.ConnectionState{}, err
	}
	cs := connection.(*tls.Conn).ConnectionState()
	connection.Close()
	return cs, nil
}

func FirstCertificate(ctx context.Context, target *url.URL) (string, error) {
	cs, err := inspectTLS(ctx, target)
	if err != nil {
		return "", err
	}
	if len(cs.PeerCertificates) == 0 {
		return "", errors.New("目标未返回 TLS 证书")
	}
	fingerprint := Fingerprint(cs.PeerCertificates[0])
	return fingerprint, VerifyPinned(cs, fingerprint)
}

func Probe(ctx context.Context, route domain.GatewayRoute) ProbeResult {
	result := ProbeResult{Target: route.Target}
	target, parseErr := url.Parse(route.Target)
	if parseErr != nil || target == nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		result.Code = "invalid_target"
		result.Message = "转发地址不合法"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if target.Scheme == "https" {
		cs, err := inspectTLS(ctx, target)
		if err != nil {
			result.Code, result.Message = ErrorReason(err)
			return result
		}
		if len(cs.PeerCertificates) == 0 {
			result.Code = "tls_error"
			result.Message = "目标未返回 TLS 证书"
			return result
		}
		cert := cs.PeerCertificates[0]
		names := append([]string{}, cert.DNSNames...)
		for _, ip := range cert.IPAddresses {
			names = append(names, ip.String())
		}
		result.Certificate = &Certificate{Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), Fingerprint: Fingerprint(cert), NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, Names: names}
		var verifyErr error
		if route.TLSFingerprint != "" {
			verifyErr = VerifyPinned(cs, route.TLSFingerprint)
		} else {
			intermediates := x509.NewCertPool()
			for _, item := range cs.PeerCertificates[1:] {
				intermediates.AddCert(item)
			}
			_, verifyErr = cert.Verify(x509.VerifyOptions{DNSName: target.Hostname(), Intermediates: intermediates})
		}
		if verifyErr != nil {
			result.Code, result.Message = ErrorReason(verifyErr)
			now := time.Now()
			result.CanTrust = !now.Before(cert.NotBefore) && !now.After(cert.NotAfter) && (result.Code == "certificate_untrusted" || result.Code == "certificate_name_mismatch" || result.Code == "certificate_changed")
			return result
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, route.Target, nil)
	if err != nil {
		result.Code = "invalid_target"
		result.Message = "转发地址不合法"
		return result
	}
	client := &http.Client{Transport: Transport(route), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		result.Code, result.Message = ErrorReason(err)
		return result
	}
	response.Body.Close()
	result.HTTPStatus = response.StatusCode
	result.Code = "connected"
	result.Message = "连接成功"
	if response.StatusCode >= 400 {
		result.Message = "已连接，目标返回 HTTP " + response.Status
	}
	return result
}

func ErrorReason(err error) (string, string) {
	if errors.Is(err, ErrFingerprint) {
		return "certificate_changed", "目标证书已更换，与已信任指纹不匹配，请重新检测并确认"
	}
	if errors.Is(err, ErrValidity) {
		return "certificate_expired", "目标证书已过期或尚未生效"
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		if invalid.Reason == x509.Expired {
			return "certificate_expired", "目标证书已过期或尚未生效"
		}
		return "certificate_invalid", "目标 TLS 证书无效"
	}
	var name x509.HostnameError
	if errors.As(err, &name) {
		return "certificate_name_mismatch", "证书名称与目标主机不匹配"
	}
	var authority x509.UnknownAuthorityError
	if errors.As(err, &authority) {
		return "certificate_untrusted", "目标证书不受信任，可能使用了自签名证书"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns_error", "无法解析目标主机名"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused", "目标拒绝连接，请检查服务是否启动及端口是否正确"
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "timeout", "连接或响应超时，请检查 NAS 到目标的网络"
	}
	var record tls.RecordHeaderError
	if errors.As(err, &record) {
		return "protocol_mismatch", "TLS 握手失败，请确认目标端口使用 HTTPS"
	}
	return "connection_failed", "无法连接目标服务，请检查网络、端口和 TLS 协议"
}
