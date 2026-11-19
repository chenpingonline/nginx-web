package httpapi

import (
	"compress/gzip"
	"encoding/json"
	"github.com/chenpingonline/nginx-web/internal/service"
	webassets "github.com/chenpingonline/nginx-web/web"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGatewayCompatibilityHTMLAndScriptRewrite(t *testing.T) {
	prefix := "/app/nginx-web/proxy/router"
	html, err := rewriteGatewayHTML(`<html><head><script type="module" src="/static/js/index.js"></script><link href="//cdn.example/style.css" rel="stylesheet"></head><body><a href="/login">Login</a></body></html>`, prefix, "http://router.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`src="` + prefix + `/__gateway_compat.js"`, `src="` + prefix + `/static/js/index.js"`, `href="` + prefix + `/login"`, `href="//cdn.example/style.css"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %s in %s", expected, html)
		}
	}
	if strings.Index(html, "__gateway_compat.js") > strings.Index(html, "/static/js/index.js") {
		t.Fatal("bootstrap runs after application script")
	}
	source := "/* __vite__mapDeps vite:preloadError */const router=make(routes.map(e=>wrap({element:render({children:e.element})})).concat(extra));const preload=function(e){return\"/\"+e};fetch(\"/Action/call\");window.location.href=\"/\";const routes2=[{path:\"/login\"}];"
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/javascript"}, "Etag": []string{"old"}}, Body: io.NopCloser(strings.NewReader(source))}
	if err := rewriteGatewayResponse(response, prefix, "http://router.test"); err != nil {
		t.Fatal(err)
	}
	rewritten, _ := io.ReadAll(response.Body)
	for _, expected := range []string{`make(routes.map(e=>wrap({element:render({children:e.element})})).concat(extra))`, `return"/"+e`, `fetch("/Action/call")`, `window.location.href="` + prefix + `/"`, `path:"/login"`} {
		if !strings.Contains(string(rewritten), expected) {
			t.Fatalf("missing %s in %s", expected, rewritten)
		}
	}
	if response.ContentLength != int64(len(rewritten)) || response.Header.Get("ETag") != "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("stale transformed response metadata")
	}
}

func TestGatewayCompatibilityRewriterHandlesCompressedCSSAndLeavesJSON(t *testing.T) {
	var body strings.Builder
	compressed := gzip.NewWriter(&body)
	compressed.Write([]byte(`a{background:url(/static/images/bg.png)}`))
	compressed.Close()
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/css"}, "Content-Encoding": []string{"gzip"}}, Body: io.NopCloser(strings.NewReader(body.String()))}
	if err := rewriteGatewayResponse(response, "/gateway/test", "http://router.test"); err != nil {
		t.Fatal(err)
	}
	actual, _ := io.ReadAll(response.Body)
	if string(actual) != `a{background:url(/gateway/test/static/images/bg.png)}` || response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("bad CSS rewrite: %s", actual)
	}
	original := `{"message":"/Action/call"}`
	response = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(original))}
	if err := rewriteGatewayResponse(response, "/gateway/test", "http://router.test"); err != nil {
		t.Fatal(err)
	}
	actual, _ = io.ReadAll(response.Body)
	if string(actual) != original {
		t.Fatal("rewrote API data")
	}
}

func TestGatewayCompatibilityGzipOnlyStaticFiles(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-javascript")
		w.Header().Set("Content-Encoding", "gzip")
		compressed := gzip.NewWriter(w)
		defer compressed.Close()
		compressed.Write([]byte(`fetch("/Action/call")`))
	}))
	defer upstream.Close()
	_, svc, paths := gatewayTestAPI(t, upstream.URL)
	state := svc.State()
	state.GatewayRoutes[0].Compatibility = "subpath"
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(paths.StateFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.AppliedState(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(paths)
	if err != nil {
		t.Fatal(err)
	}
	api := New(svc, webassets.Assets)
	response := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/static/js/app.js", "")
	if response.Code != 200 || response.Body.String() != `fetch("/Action/call")` || response.Header().Get("Content-Encoding") != "" {
		t.Fatalf("gzip-only resource failed: %d %s", response.Code, response.Body.String())
	}
}

func TestGatewayCompatibilityResourcesAcrossDeviceLayouts(t *testing.T) {
	prefix, target := "/app/nginx-web/proxy/camera", "https://192.168.1.50:8443/admin"
	for _, tc := range []struct{ in, want string }{
		{"/web/ui.js", prefix + "/web/ui.js"},
		{"/cgi-bin/status?x=1#video", prefix + "/cgi-bin/status?x=1#video"},
		{"https://192.168.1.50:8443/admin/api/status", prefix + "/api/status"},
		{"//192.168.1.50:8443/admin/icons/a.svg", prefix + "/icons/a.svg"},
		{"https://cdn.example/web/ui.js", "https://cdn.example/web/ui.js"},
		{"//cdn.example/web/ui.js", "//cdn.example/web/ui.js"},
		{"#/network", "#/network"}, {"?view=1", "?view=1"},
		{"../img/a.png", "../img/a.png"}, {"data:image/png;base64,AA", "data:image/png;base64,AA"},
		{prefix + "/web/ui.js", prefix + "/web/ui.js"},
		{"/api/a%2Fb", prefix + "/api/a%2Fb"},
	} {
		if got := gatewayResourceURL(tc.in, prefix, target); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.in, got, tc.want)
		}
	}
	source := `<head><base href="/admin/"><meta http-equiv="refresh" content="1;url=/signin"><style>.x{background:url('/images/a.png')} @import "/themes/dark.css";</style><script type="application/json">{"url":"/static/data"}</script></head><body><img srcset="/images/a.png 1x, /images/b.png 2x"><form action="/cgi/login"><button formaction="/cgi/logout">out</button></form><script>fetch('/api/login');window.location.href='/start';</script><a href="#/network">Network</a></body>`
	got, err := rewriteGatewayHTML(source, prefix, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="` + prefix + `/"`, `content="1;url=` + prefix + `/signin"`, `url('` + prefix + `/images/a.png')`, `@import "` + prefix + `/themes/dark.css"`, `/images/b.png 2x`, `action="` + prefix + `/cgi/login"`, `formaction="` + prefix + `/cgi/logout"`, `{"url":"/static/data"}`, `fetch('/api/login')`, `window.location.href='` + prefix + `/start'`, `href="#/network"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestGatewayCompatibilityDoesNotRewriteBusinessStrings(t *testing.T) {
	prefix := "/app/nginx-web/proxy/router"
	source := `const message='location.href="/static/help"';/* location.href="/comments" */
// location.href="/comments"
const routes=[{path:"/login"}],external="https://cdn.example/static/a.js",payload={url:"/Action/call"};
window.location.href="/signin";import("/assets/page.js");export {page} from "/modules/page.js";`
	got := rewriteGatewayJS(source, prefix, "http://router.test")
	for _, want := range []string{`message='location.href="/static/help"'`, `/* location.href="/comments" */`, `// location.href="/comments"`, `path:"/login"`, `https://cdn.example/static/a.js`, `url:"/Action/call"`, `window.location.href="` + prefix + `/signin"`, `import("` + prefix + `/assets/page.js")`, `from "` + prefix + `/modules/page.js"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestGatewayCompatibilityLegacyDeviceCharset(t *testing.T) {
	source := `<html><head><meta charset="gbk"><meta http-equiv="Content-Type" content="text/html; charset=gbk"></head><body>摄像头管理<img src="/images/logo.png"></body></html>`
	encoded, _, err := transform.String(simplifiedchinese.GBK.NewEncoder(), source)
	if err != nil {
		t.Fatal(err)
	}
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html; charset=gbk"}}, Body: io.NopCloser(strings.NewReader(encoded))}
	if err := rewriteGatewayResponse(response, "/proxy/camera", "http://camera.test"); err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(data), "摄像头管理") || strings.Contains(string(data), "gbk") || response.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("invalid converted page: %s", data)
	}
}
