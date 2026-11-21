package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/gatewayclient"
	"github.com/chenpingonline/nginx-web/internal/platform"
	"github.com/chenpingonline/nginx-web/internal/service"
	webassets "github.com/chenpingonline/nginx-web/web"
)

func gatewayTestAPI(t *testing.T, target string) (*API, *service.AppService, platform.Paths) {
	t.Helper()
	root := t.TempDir()
	for key, dir := range map[string]string{"APPDEST": "app", "ETC": "etc", "VAR": "var", "TMP": "tmp"} {
		t.Setenv("FNPROXY_"+key, filepath.Join(root, dir))
	}
	t.Setenv("FNPROXY_DEV_ALLOW", "0")
	paths, err := platform.LoadPaths()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(paths)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveGatewayRoute("", domain.GatewayRoute{Name: "test", Path: "music", Target: target, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return New(svc, webassets.Assets), svc, paths
}

func gatewayRequest(api *API, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Trim-Isadmin", "true")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	return w
}

func TestGatewayDiscoveryRequiresAdminAndLeavesDraftUntouched(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "music")
	if err := os.MkdirAll(filepath.Join(base, "target/ui"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "manifest"), []byte("display_name=Music"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "target/ui/config"), []byte(`{".url":{"music.main":{"title":"Music","type":"url","protocol":"http","port":"4533"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FNPROXY_APP_REGISTRY", root)
	api, svc, _ := gatewayTestAPI(t, "http://127.0.0.1:4533")
	before, _ := json.Marshal(svc.State())
	unauthorized := httptest.NewRecorder()
	api.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/gateway-apps", nil))
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("unauthorized discovery: %d", unauthorized.Code)
	}
	w := gatewayRequest(api, http.MethodGet, "/api/gateway-apps", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"existing":true`) {
		t.Fatalf("discovery failed: %d %s", w.Code, w.Body.String())
	}
	after, _ := json.Marshal(svc.State())
	if string(before) != string(after) {
		t.Fatal("read-only discovery modified draft")
	}
	standalone := NewStandalone(svc, webassets.Assets)
	w = gatewayRequest(standalone, http.MethodGet, "/api/gateway-apps", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("standalone must not trust NAS identity headers: %d", w.Code)
	}
}

func TestGatewayForwardsPathQueryBodyAndIsolatesCredentials(t *testing.T) {
	var got *http.Request
	var body string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "upstream", Domain: "internal.test", Path: "/", HttpOnly: true})
		w.Header().Set("Location", "/base/login?q=1")
		w.WriteHeader(302)
	}))
	defer backend.Close()
	api, svc, _ := gatewayTestAPI(t, backend.URL+"/base")
	id := svc.State().GatewayRoutes[0].ID
	r := httptest.NewRequest("POST", gatewayPrefix+"/proxy/music/api/a%2Fb?q=hello%20world", strings.NewReader("payload"))
	r.Header.Set("X-Trim-Isadmin", "true")
	r.Header.Set("X-Trim-Username", "nas-admin")
	r.Header.Set("X-FnProxy-Request", "1")
	r.Header.Set("Authorization", "Bearer nas-secret")
	r.Header.Set("Cookie", "nas_session=private; ngw_"+id+"_session=backend-session; unrelated=secret")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != 302 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if got.URL.EscapedPath() != "/base/api/a%2Fb" || got.URL.RawQuery != "q=hello%20world" || body != "payload" {
		t.Fatalf("incorrect forward: %s %q", got.URL, body)
	}
	for _, name := range []string{"X-Trim-Isadmin", "X-Trim-Username", "X-FnProxy-Request", "Authorization"} {
		if got.Header.Get(name) != "" {
			t.Fatalf("leaked %s", name)
		}
	}
	if got.Header.Get("Cookie") != "session=backend-session" {
		t.Fatalf("leaked cookie: %s", got.Header.Get("Cookie"))
	}
	if got.Header.Get("X-Forwarded-Prefix") != gatewayPrefix+"/proxy/music" {
		t.Fatal("missing prefix")
	}
	if w.Header().Get("Location") != gatewayPrefix+"/proxy/music/login?q=1" {
		t.Fatalf("redirect: %s", w.Header().Get("Location"))
	}
	c := w.Result().Cookies()[0]
	if c.Name != "ngw_"+id+"_session" || c.Domain != "" || c.Path != gatewayPrefix+"/proxy/music/" || !c.HttpOnly {
		t.Fatalf("cookie isolation: %+v", c)
	}
	if w.Header().Get("Content-Security-Policy") != "" {
		t.Fatal("management CSP leaked into upstream response")
	}
}

func TestGatewayAuthorizationDraftDeletionAndRestart(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "real-upstream") }))
	defer backend.Close()
	api, svc, paths := gatewayTestAPI(t, backend.URL)
	r := httptest.NewRequest("GET", gatewayPrefix+"/proxy/music/", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("unauthorized: %d", w.Code)
	}
	if got := gatewayRequest(NewStandalone(svc, webassets.Assets), "GET", gatewayPrefix+"/proxy/music/", ""); got.Code != 404 {
		t.Fatal("standalone exposed gateway")
	}
	for _, path := range []string{"missing/", "music/../api/overview", "music/%2e%2e/api", "music/a%5Cb"} {
		w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/"+path, "")
		if w.Code != 400 && w.Code != 404 {
			t.Fatalf("unsafe path %s: %d", path, w.Code)
		}
	}
	if w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music?x=1", ""); w.Code != 307 || w.Header().Get("Location") != gatewayPrefix+"/proxy/music/?x=1" {
		t.Fatal("slash redirect lost query")
	}
	if err := svc.DeleteGatewayRoute(svc.State().GatewayRoutes[0].ID); err != nil {
		t.Fatal(err)
	}
	if w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/", ""); w.Code != http.StatusNotFound {
		t.Fatal("deleted route still forwarded")
	}
	restarted, err := service.New(paths)
	if err != nil {
		t.Fatal(err)
	}
	if w := gatewayRequest(New(restarted, webassets.Assets), "GET", gatewayPrefix+"/proxy/music/", ""); w.Code != http.StatusNotFound {
		t.Fatal("restart restored deleted route")
	}
	backup, err := svc.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.State.GatewayRoutes) != 0 {
		t.Fatal("backup not using current draft")
	}
}

func TestGatewayStreamsImmediatelyAndReportsUnavailableUpstream(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: hello\n\n")
		w.(http.Flusher).Flush()
	}))
	api, _, _ := gatewayTestAPI(t, backend.URL)
	w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/", "")
	if w.Code != 200 || !w.Flushed || !strings.Contains(w.Body.String(), "hello") {
		t.Fatal("stream response not flushed")
	}
	backend.Close()
	if w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/", ""); w.Code != 502 {
		t.Fatalf("upstream error: %d", w.Code)
	}
}

func TestGatewayWebSocketUpgradeAndEcho(t *testing.T) {
	upgrader := websocket.Upgrader{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		kind, data, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(kind, append([]byte("upstream:"), data...))
		}
	}))
	defer backend.Close()
	api, _, _ := gatewayTestAPI(t, backend.URL)
	gateway := httptest.NewServer(api)
	defer gateway.Close()
	headers := http.Header{"X-Trim-Isadmin": []string{"true"}, "Origin": []string{gateway.URL}}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+gatewayPrefix+"/proxy/music/ws", headers)
	if err != nil {
		t.Fatalf("upgrade failed: %v (%v)", err, response)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil || string(data) != "upstream:ping" {
		t.Fatalf("echo: %q %v", data, err)
	}
}

func TestGatewayRedirectOnlyStripsCompleteBasePath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/baseball/login?next=1")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()
	api, _, _ := gatewayTestAPI(t, backend.URL+"/base")
	w := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/", "")
	if got := w.Header().Get("Location"); got != gatewayPrefix+"/proxy/music/baseball/login?next=1" {
		t.Fatalf("partial base path stripped: %s", got)
	}
}

func TestGatewayPreservesRedirectAlreadyUsingPublicPrefix(t *testing.T) {
	prefix := gatewayPrefix + "/proxy/music"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", prefix+"/user/login?redirect_to=%2Fdashboard")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()
	api, _, _ := gatewayTestAPI(t, backend.URL)
	w := gatewayRequest(api, "GET", prefix+"/user/settings", "")
	if got := w.Header().Get("Location"); got != prefix+"/user/login?redirect_to=%2Fdashboard" {
		t.Fatalf("public prefix duplicated: %s", got)
	}
}

func TestGatewayRelativeRedirectStaysInsideSelectedService(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "../signin?next=panel")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	api, _, _ := gatewayTestAPI(t, upstream.URL+"/base")
	response := gatewayRequest(api, "GET", gatewayPrefix+"/proxy/music/nested/panel", "")
	if got := response.Header().Get("Location"); got != gatewayPrefix+"/proxy/music/signin?next=panel" {
		t.Fatalf("relative redirect escaped: %s", got)
	}
}

func TestGatewayFirstVisitPinsBeforeForwarding(t *testing.T) {
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	api, svc, paths := gatewayTestAPI(t, upstream.URL)
	const path = gatewayPrefix + "/proxy/music/"
	if w := gatewayRequest(api, "GET", path, ""); w.Code != http.StatusBadGateway || calls != 0 {
		t.Fatal("default trust policy", w.Code, calls)
	}
	route, _ := svc.GatewayRoute("music")
	route.TrustFirstCertificate = true
	if _, err := svc.SaveGatewayRoute(route.ID, route); err != nil {
		t.Fatal(err)
	}
	if w := gatewayRequest(api, "GET", path, ""); w.Code != http.StatusNoContent || calls != 1 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
	restarted, err := service.New(paths)
	if err != nil {
		t.Fatal(err)
	}
	pinned, ok := restarted.GatewayRoute("music")
	if !ok || pinned.TrustFirstCertificate || pinned.TLSFingerprint != gatewayclient.Fingerprint(upstream.Certificate()) || pinned.TLSTrustTarget != upstream.URL {
		t.Fatal(pinned)
	}
	// A different saved pin must fail closed without recording the peer again.
	pinned.TLSFingerprint = strings.Repeat("0", 64)
	if _, err := svc.SaveGatewayRoute(pinned.ID, pinned); err != nil {
		t.Fatal(err)
	}
	if w := gatewayRequest(api, "GET", path, ""); w.Code != http.StatusBadGateway || calls != 1 || !strings.Contains(w.Body.String(), "certificate_changed") {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
}

func TestGatewayEntryOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, host, proto, referer, want string
		socket                           bool
	}{
		{"LAN", "192.168.1.88:5666", "http", "", "http", true},
		{"fnConnect", "nas.example", "https", "", "https", true},
		{"same-origin fallback", "nas.example", "", "https://nas.example/app/", "https", true},
		{"foreign referrer", "nas.example", "", "https://other.example/", "http", true},
		{"untrusted TCP", "nas.example", "https", "", "http", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != tc.host || r.Header.Get("X-Forwarded-Host") != tc.host || r.Header.Get("X-Forwarded-Proto") != tc.want {
					t.Errorf("upstream host=%q forwarded=%q proto=%q", r.Host, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"))
				}
				http.Redirect(w, r, tc.want+"://"+tc.host+"/user/login", http.StatusFound)
			}))
			defer backend.Close()
			api, _, _ := gatewayTestAPI(t, backend.URL)
			r := httptest.NewRequest("GET", gatewayPrefix+"/proxy/music/", nil)
			r.Host = tc.host
			r.Header.Set("X-Trim-Isadmin", "true")
			r.Header.Set("X-Forwarded-Proto", tc.proto)
			r.Header.Set("Referer", tc.referer)
			if tc.socket {
				r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.UnixAddr{Name: "app.sock", Net: "unix"}))
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if got := w.Header().Get("Location"); got != gatewayPrefix+"/proxy/music/user/login" {
				t.Errorf("redirect = %q", got)
			}
		})
	}
}

func TestGatewayBrowserEntryRestoresFnosOrigin(t *testing.T) {
	for _, entry := range []string{"http://192.168.1.88:5666", "https://nas.example"} {
		t.Run(entry, func(t *testing.T) {
			calls := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("X-Forwarded-Proto")+"://"+r.Host != entry || r.Header.Get("X-Forwarded-Host") != r.Host || r.Header.Get("Cookie") != "" {
					t.Errorf("incorrect upstream origin or cookie isolation")
				}
				http.Redirect(w, r, entry+"/dashboard", http.StatusFound)
			}))
			defer backend.Close()
			api, svc, _ := gatewayTestAPI(t, backend.URL)
			route := svc.State().GatewayRoutes[0]
			route.Compatibility = "subpath"
			if _, err := svc.SaveGatewayRoute(route.ID, route); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", gatewayPrefix+"/proxy/music/", nil)
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.UnixAddr{Net: "unix"}))
			r.Host = "localhost"
			r.Header.Set("X-Trim-Isadmin", "true")
			r.Header.Set("Accept", "text/html")
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if calls != 0 || w.Code != 200 || !strings.Contains(w.Body.String(), "location.origin") {
				t.Fatal("missing entry bootstrap")
			}
			r.AddCookie(&http.Cookie{Name: "fnproxy_gateway_entry", Value: entry})
			w = httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if calls != 1 || w.Header().Get("Location") != gatewayPrefix+"/proxy/music/dashboard" {
				t.Fatal("incorrect entry redirect")
			}
		})
	}
}
