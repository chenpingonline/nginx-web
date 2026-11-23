package nginx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

// FormConfig uses the same renderers as deployment. Some forms own directives
// at several scopes, and application-only properties are carried by the exporter.
func (m *Manager) FormConfig(state State, kind, id string) (string, error) {
	switch kind {
	case "rules", "streams":
		return m.RuleConfig(state, id, kind == "streams")
	case "rule-groups":
		for _, g := range state.RuleGroups {
			if g.ID == id {
				var b strings.Builder
				suffix := ""
				if g.TLS {
					suffix = " ssl"
				}
				writeHTTPListeners(&b, g.ListenType, g.ListenPort, suffix)
				if g.TLS && g.HTTP2 {
					b.WriteString("    http2 on;\n")
				}
				if g.TLS {
					cert, key := m.certificateFiles(g.CertificateID)
					fmt.Fprintf(&b, "    ssl_certificate %s;\n    ssl_certificate_key %s;\n", nginxQuote(cert), nginxQuote(key))
				}
				return b.String(), nil
			}
		}
	case "settings":
		isolated := domain.DefaultState()
		isolated.Settings = state.Settings
		master, _, err := m.render(isolated, m.paths.NginxConfD)
		if err != nil {
			return "", err
		}
		prelude := strings.TrimSpace(renderHTTPPrelude(isolated, m.paths.NginxCacheDir))
		if prelude != "" {
			master += "\n# 001-runtime-and-upstreams.conf\n# " + strings.ReplaceAll(prelude, "\n", "\n# ") + "\n"
		}
		return master, nil
	case "upstreams":
		for _, pool := range state.UpstreamPools {
			if pool.ID == id {
				scope := State{UpstreamPools: []domain.UpstreamPool{pool}}
				if pool.Protocol == "stream" {
					return renderStreamPrelude(scope), nil
				}
				return renderHTTPPrelude(scope, m.paths.NginxCacheDir), nil
			}
		}
	case "rate-limits":
		for _, p := range state.RateLimitPolicies {
			if p.ID == id {
				limits := p.Settings
				limits.Enabled = true
				r := ProxyRule{ID: id, Enabled: true, RateLimit: limits}
				var b strings.Builder
				b.WriteString(renderHTTPPrelude(State{Rules: []ProxyRule{r}}, m.paths.NginxCacheDir))
				renderRateLimits(&b, r)
				return b.String(), nil
			}
		}
	case "auth-profiles":
		for _, p := range state.AuthProfiles {
			if p.ID == id {
				return fmt.Sprintf("# 认证服务引用\nproxy_pass %s;\n", nginxQuote("http://unix:"+m.paths.SocketPath+":/internal/basic-auth/"+id)), nil
			}
		}
	case "waf":
		for _, p := range state.WAFInstances {
			if p.ID == id {
				var b strings.Builder
				for _, site := range p.Sites {
					for _, r := range state.Rules {
						if r.ID == site {
							fmt.Fprintf(&b, "# server %s\n", r.ID)
							if err := m.writeWAF(&b, p, r); err != nil {
								return "", err
							}
						}
					}
				}
				if b.Len() == 0 {
					b.WriteString("# 尚未绑定站点\n")
				}
				return b.String(), nil
			}
		}
	}
	return "", errors.New("找不到要导出的表单")
}
