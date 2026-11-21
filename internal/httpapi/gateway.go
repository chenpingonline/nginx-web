package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/gatewayclient"
)

// TLS terminates at the fnOS frontend before it forwards requests over the app socket.
func gatewayEntryScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr.Network() == "unix" {
		switch r.Header.Get("X-Forwarded-Proto") {
		case "http", "https":
			return r.Header.Get("X-Forwarded-Proto")
		}
		for _, header := range []string{"Origin", "Referer"} {
			u, err := url.Parse(r.Header.Get(header))
			if err == nil && strings.EqualFold(u.Host, r.Host) && (u.Scheme == "https" || u.Scheme == "http") {
				return u.Scheme
			}
		}
	}
	return "http"
}

func browserGatewayOrigin(value string) *url.URL {
	origin, err := url.Parse(value)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Hostname() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.String() != origin.Scheme+"://"+origin.Host {
		return nil
	}
	return origin
}

func (a *API) serveGatewayProxy(w http.ResponseWriter, r *http.Request) {
	if a.standalone {
		http.NotFound(w, r)
		return
	}
	if !a.requireAdmin(w, r) {
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" && r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeAPIError(w, http.StatusForbidden, "不允许跨站提交网关请求")
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, gatewayPrefix+"/proxy/")
	slug, suffix, hasSlash := strings.Cut(tail, "/")
	route, ok := a.service.GatewayRoute(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Do not let path normalization escape a selected upstream base path.
	if strings.ContainsAny(suffix, "\\\x00") {
		writeAPIError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	for _, part := range strings.Split(suffix, "/") {
		if part == "." || part == ".." {
			writeAPIError(w, http.StatusBadRequest, "路径不合法")
			return
		}
	}
	prefix := gatewayPrefix + "/proxy/" + slug
	if !hasSlash {
		dest := prefix + "/"
		if r.URL.RawQuery != "" {
			dest += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
		return
	}
	target, err := url.Parse(route.Target)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "网关目标不合法")
		return
	}
	cookiePrefix := "ngw_" + route.ID + "_"
	if route.UsesSubpathCompatibility() && (suffix == "__gateway_compat.js" || suffix == "__gateway_ikuai.js") {
		serveGatewayBootstrap(w, prefix, cookiePrefix, route.Target)
		return
	}
	if target.Scheme == "https" && route.TrustFirstCertificate && route.TLSFingerprint == "" {
		fingerprint, err := gatewayclient.FirstCertificate(r.Context(), target)
		if err != nil {
			code, message := gatewayclient.ErrorReason(err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": message, "code": code})
			return
		}
		route, err = a.service.PinFirstGatewayCertificate(route, fingerprint)
		if err != nil {
			writeAPIError(w, http.StatusConflict, err.Error())
			return
		}
	}
	entry := url.URL{Scheme: gatewayEntryScheme(r), Host: r.Host, Path: target.Path}
	// fnOS can replace Host with localhost (fnConnect) or drop the LAN port.
	// The browser knows its origin even for bookmarks without a Referer.
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr.Network() == "unix" {
		const entryCookie = "fnproxy_gateway_entry"
		if cookie, err := r.Cookie(entryCookie); err == nil {
			if origin := browserGatewayOrigin(cookie.Value); origin != nil {
				entry.Host, entry.Scheme = origin.Host, origin.Scheme
			}
		}
		if route.UsesSubpathCompatibility() && r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") && (r.Header.Get("Sec-Fetch-Mode") == "" || r.Header.Get("Sec-Fetch-Mode") == "navigate") {
			cookie, err := r.Cookie(entryCookie)
			if err != nil || browserGatewayOrigin(cookie.Value) == nil {
				path, _ := json.Marshal(prefix + "/")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "private, no-store")
				fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>打开网关服务</title><script>
const cookie = "fnproxy_gateway_entry=" + location.origin;
document.cookie = cookie + "; Path=" + %s + "; SameSite=Lax" + (location.protocol === "https:" ? "; Secure" : "");
if (document.cookie.split("; ").includes(cookie)) location.replace(location.href);
else document.addEventListener("DOMContentLoaded", () => document.body.textContent = "请允许此站点使用 Cookie 后刷新。");
</script><noscript>请启用 JavaScript 后刷新以打开此网关服务。</noscript>`, path)
				return
			}
		}
	}
	proxy := &httputil.ReverseProxy{
		Transport:     gatewayclient.Transport(route),
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if route.UsesSubpathCompatibility() {
				// Some devices publish only gzip static assets; decode before adapting text.
				pr.Out.Header.Set("Accept-Encoding", "gzip")
				pr.Out.Header.Del("If-None-Match")
				pr.Out.Header.Del("If-Modified-Since")
			}
			// Keep escaped path semantics (e.g. %2F) after removing only our prefix.
			pr.Out.URL.Path = strings.TrimRight(target.Path, "/") + "/" + suffix
			raw := strings.TrimPrefix(pr.In.URL.EscapedPath(), prefix+"/")
			pr.Out.URL.RawPath = strings.TrimRight(target.EscapedPath(), "/") + "/" + raw
			if unescaped, err := url.PathUnescape(pr.Out.URL.RawPath); err != nil || unescaped != pr.Out.URL.Path {
				pr.Out.URL.RawPath = ""
			}
			pr.Out.Host = entry.Host
			for name := range pr.Out.Header {
				lower := strings.ToLower(name)
				if strings.HasPrefix(lower, "x-trim-") || strings.HasPrefix(lower, "x-fnproxy-") || strings.HasPrefix(lower, "x-forwarded-") || lower == "forwarded" || lower == "x-real-ip" {
					pr.Out.Header.Del(name)
				}
			}
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Proxy-Authorization")
			// Each route has its own cookie namespace. NAS sessions never reach upstreams.
			for _, c := range pr.In.Cookies() {
				if strings.HasPrefix(c.Name, cookiePrefix) {
					pr.Out.AddCookie(&http.Cookie{Name: strings.TrimPrefix(c.Name, cookiePrefix), Value: c.Value})
				}
			}
			pr.Out.Header.Set("X-Forwarded-Prefix", prefix)
			pr.Out.Header.Set("X-Forwarded-Host", entry.Host)
			pr.Out.Header.Set("X-Forwarded-Proto", entry.Scheme)
			if host, _, err := net.SplitHostPort(pr.In.RemoteAddr); err == nil {
				pr.Out.Header.Set("X-Forwarded-For", host)
			}
			// Origin stays on the browser origin for upstream CSRF checks.
			if ref, err := url.Parse(pr.In.Header.Get("Referer")); err == nil && strings.EqualFold(ref.Host, entry.Host) && ref.Scheme == entry.Scheme && strings.HasPrefix(ref.Path, prefix+"/") {
				ref.Path = strings.TrimRight(target.Path, "/") + strings.TrimPrefix(ref.Path, prefix)
				ref.RawPath = ""
				pr.Out.Header.Set("Referer", ref.String())
			} else {
				pr.Out.Header.Del("Referer")
			}
		},
		ModifyResponse: func(res *http.Response) error {
			if value := res.Header.Get("Location"); value != "" {
				if u, err := url.Parse(value); err == nil {
					if u.Path != "" && !strings.HasPrefix(u.Path, "/") {
						requestURL := *target
						requestURL.Path = strings.TrimRight(target.Path, "/") + "/" + suffix
						u = requestURL.ResolveReference(u)
					}
					res.Header.Set("Location", gatewayResourceURL(u.String(), prefix, route.Target, entry.String()))
				}
			}

			cookies := res.Cookies()
			res.Header.Del("Set-Cookie")
			for _, c := range cookies {
				c.Name = cookiePrefix + c.Name
				c.Domain = ""
				c.Path = prefix + "/"
				res.Header.Add("Set-Cookie", c.String())
			}
			if route.UsesSubpathCompatibility() {
				return rewriteGatewayResponse(res, prefix, route.Target, entry.String())
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() != context.Canceled {
				code, message := gatewayclient.ErrorReason(err)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": message, "code": code})
			}
		},
	}
	proxy.ServeHTTP(w, r)
}
