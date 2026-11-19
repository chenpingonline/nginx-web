package domain

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// GatewayRoute is an explicit administrator-controlled HTTP destination.
// Paths are one segment under /app/nginx-web/proxy/, never arbitrary URLs.
type GatewayRoute struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Target         string `json:"target"`
	Enabled        bool   `json:"enabled"`
	Compatibility  string `json:"compatibility,omitempty"`
	TLSFingerprint string `json:"tls_fingerprint,omitempty"`
	TLSTrustTarget string `json:"tls_trust_target,omitempty"`
}

var gatewayPathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

func NormalizeGatewayRoute(r *GatewayRoute) {
	r.TLSFingerprint = strings.ToLower(strings.TrimSpace(r.TLSFingerprint))
	r.Name = strings.TrimSpace(r.Name)
	r.Path = strings.Trim(strings.ToLower(strings.TrimSpace(r.Path)), "/")
	r.Target = strings.TrimRight(strings.TrimSpace(r.Target), "/")
}

func ValidateGatewayRoutes(routes []GatewayRoute) error {
	if len(routes) > 128 {
		return errors.New("网关服务最多支持 128 条")
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, r := range routes {
		if !ValidID(r.ID) || ids[r.ID] {
			return errors.New("网关规则 ID 不合法或重复")
		}
		ids[r.ID] = true
		if len([]rune(r.Name)) < 1 || len([]rune(r.Name)) > 80 {
			return errors.New("网关规则名称长度必须为 1 到 80 个字符")
		}
		if !gatewayPathPattern.MatchString(r.Path) {
			return errors.New("网关路径需为 1 到 48 个小写字母、数字或连字符，首位不能为连字符")
		}
		if paths[r.Path] {
			return fmt.Errorf("网关路径 %q 已被使用", r.Path)
		}
		paths[r.Path] = true
		u, err := url.Parse(r.Target)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
			return errors.New("转发地址必须是 HTTP(S) 地址，不能包含账号、查询参数或片段")
		}
		if r.Compatibility != "" && r.Compatibility != "subpath" && r.Compatibility != "ikuai-v4" {
			return errors.New("不支持的网关页面适配模式")
		}
		if r.TLSFingerprint != "" || r.TLSTrustTarget != "" {
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(r.TLSFingerprint) || u.Scheme != "https" || r.TLSTrustTarget != GatewayTrustTarget(r.Target) {
				return errors.New("证书信任指纹或目标不匹配，请重新检测该地址的证书")
			}
		}
		if err := validateHostName(u.Hostname(), false); err != nil {
			return errors.New("网关目标主机不合法")
		}
		if u.Port() != "" {
			var port int
			if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil || port < 1 || port > 65535 {
				return errors.New("网关目标端口必须为 1 到 65535")
			}
		}
		if strings.ContainsAny(u.Path, "\\\r\n") || strings.Contains(u.EscapedPath(), "%") {
			return errors.New("目标基础路径不支持转义字符或反斜杠")
		}
		for _, part := range strings.Split(u.Path, "/") {
			if part == "." || part == ".." {
				return errors.New("目标基础路径不能包含 . 或 ..")
			}
		}
	}
	return nil
}

// Trust includes the configured base path so editing a target requires reconfirmation.
func GatewayTrustTarget(target string) string {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(target), "/"))
	if err != nil || u.Scheme != "https" {
		return ""
	}
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

// Legacy iKuai routes use the same generic adapter without rewriting stored snapshots.
func (r GatewayRoute) UsesSubpathCompatibility() bool {
	return r.Compatibility == "subpath" || r.Compatibility == "ikuai-v4"
}
