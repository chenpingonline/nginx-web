package nginx

import (
	"fmt"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

// A named error-page location preserves the request method and buffered body.
// Only an allow response from the private inspector resumes location selection.
func (m *Manager) writeWAF(b *strings.Builder, p domain.WAFInstance, r ProxyRule) error {
	if !domain.ValidID(p.ID) || !domain.ValidID(r.ID) || (p.Mode != "detect" && p.Mode != "block") {
		return fmt.Errorf("WAF 实例配置无效")
	}
	if r.RedirectToHTTPS {
		b.WriteString("    error_page 418 = @fnproxy_inspect;\n    if ($fnproxy_waf_checked = 0) { return 418; }\n")
	}
	b.WriteString("    location @fnproxy_inspect {\n")
	m.writeInspectorProxy(b, p, r, "generic")
	b.WriteString("    }\n    location @fnproxy_checked {\n        set $fnproxy_waf_checked 1;\n")
	if r.RedirectToHTTPS {
		port := ""
		if r.RedirectHTTPSPort != 443 {
			port = fmt.Sprintf(":%d", r.RedirectHTTPSPort)
		}
		fmt.Fprintf(b, "        return 308 https://$host%s$request_uri;\n", port)
	} else {
		b.WriteString("        rewrite ^ $uri last;\n")
	}
	b.WriteString("    }\n")
	return nil
}

// X-Accel-Redirect to a named proxy location retains the original unparsed
// URI, including encoded slashes, as well as the method and buffered body.
func (m *Manager) writeInspectorProxy(b *strings.Builder, p domain.WAFInstance, r ProxyRule, target string) {
	fmt.Fprintf(b, `        auth_request off;
        proxy_request_buffering on;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Fnproxy-Mode %s;
        proxy_set_header X-Fnproxy-Instance %s;
        proxy_set_header X-Fnproxy-Site %s;
        proxy_set_header X-Fnproxy-Target %s;
        proxy_set_header X-Fnproxy-Client $remote_addr;
        proxy_set_header X-Fnproxy-Protocol $server_protocol;
        proxy_set_header Connection "";
        proxy_pass %s;
`, p.Mode, p.ID, r.ID, target, nginxQuote("http://unix:"+m.paths.WAFSocket()+":"))
}
