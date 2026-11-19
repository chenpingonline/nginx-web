package httpapi

import (
	"context"
	"github.com/chenpingonline/nginx-web/internal/gatewayclient"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

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
			pr.Out.Host = target.Host
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
			pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
			pr.Out.Header.Set("X-Forwarded-Proto", "https")
			if host, _, err := net.SplitHostPort(pr.In.RemoteAddr); err == nil {
				pr.Out.Header.Set("X-Forwarded-For", host)
			}
			// Backends that enforce their own Origin checks see their actual origin.
			if pr.Out.Header.Get("Origin") != "" {
				pr.Out.Header.Set("Origin", target.Scheme+"://"+target.Host)
			}
			if ref, err := url.Parse(pr.Out.Header.Get("Referer")); err == nil && strings.HasPrefix(ref.Path, prefix+"/") {
				ref.Scheme, ref.Host = target.Scheme, target.Host
				ref.Path = strings.TrimRight(target.Path, "/") + strings.TrimPrefix(ref.Path, prefix)
				ref.RawPath = ""
				pr.Out.Header.Set("Referer", ref.String())
			} else {
				pr.Out.Header.Del("Referer")
			}
		},
		ModifyResponse: func(res *http.Response) error {
			if value := res.Header.Get("Location"); value != "" {
				if u, err := url.Parse(value); err == nil && (u.Host == "" || strings.EqualFold(u.Host, target.Host)) {
					if u.Path != "" && !strings.HasPrefix(u.Path, "/") {
						requestURL := *target
						requestURL.Path = strings.TrimRight(target.Path, "/") + "/" + suffix
						u = requestURL.ResolveReference(u)
					}
					res.Header.Set("Location", gatewayResourceURL(u.String(), prefix, route.Target))
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
				return rewriteGatewayResponse(res, prefix, route.Target)
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
