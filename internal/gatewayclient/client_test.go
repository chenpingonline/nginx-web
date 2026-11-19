package gatewayclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestProbeAndRouteScopedTrust(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("probe sent credentials")
		}
		w.WriteHeader(200)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	route := domain.GatewayRoute{Target: server.URL}
	result := Probe(context.Background(), route)
	if result.Code != "certificate_untrusted" || !result.CanTrust || result.Certificate == nil {
		t.Fatalf("unexpected probe: %+v", result)
	}
	if requests.Load() != 0 {
		t.Fatal("untrusted diagnostic sent application traffic")
	}
	route.TLSFingerprint = result.Certificate.Fingerprint
	route.TLSTrustTarget = domain.GatewayTrustTarget(route.Target)
	result = Probe(context.Background(), route)
	if result.Code != "connected" || result.HTTPStatus != 200 {
		t.Fatalf("pin did not connect: %+v", result)
	}
	response, err := (&http.Client{Transport: Transport(route)}).Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err := (&http.Client{Transport: Transport(domain.GatewayRoute{Target: server.URL})}).Get(server.URL); err == nil {
		t.Fatal("route pin weakened default TLS policy")
	}
	route.TLSFingerprint = strings.Repeat("0", 64)
	result = Probe(context.Background(), route)
	if result.Code != "certificate_changed" || !result.CanTrust {
		t.Fatalf("changed certificate silently accepted: %+v", result)
	}
	route.TLSTrustTarget = "https://other.example.test"
	if Transport(route) != strictTransport {
		t.Fatal("pin carried over to another destination")
	}
}

func TestPinnedCertificateValidityAndErrorClassification(t *testing.T) {
	cert := &x509.Certificate{Raw: []byte("leaf"), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	if err := VerifyPinned(state, Fingerprint(cert)); err != nil {
		t.Fatal(err)
	}
	cert.NotAfter = time.Now().Add(-time.Minute)
	if !errors.Is(VerifyPinned(state, Fingerprint(cert)), ErrValidity) {
		t.Fatal("expired pin accepted")
	}
	if code, _ := ErrorReason(x509.HostnameError{Certificate: cert, Host: "router.test"}); code != "certificate_name_mismatch" {
		t.Fatal(code)
	}
	if code, _ := ErrorReason(context.DeadlineExceeded); code != "timeout" {
		t.Fatal(code)
	}
	if code, _ := ErrorReason(x509.CertificateInvalidError{Cert: cert, Reason: x509.Expired}); code != "certificate_expired" {
		t.Fatal(code)
	}
}

func TestProbeDoesNotFollowRedirects(t *testing.T) {
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("probe followed redirect") }))
	defer next.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, next.URL, 302) }))
	defer server.Close()
	result := Probe(context.Background(), domain.GatewayRoute{Target: server.URL})
	if result.Code != "connected" || result.HTTPStatus != 302 {
		t.Fatalf("unexpected redirect result: %+v", result)
	}
}
