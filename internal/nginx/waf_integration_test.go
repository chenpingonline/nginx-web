package nginx

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/waf"
)

// Exercise the generated virtual hosts and delivered upstream requests. BREW
// is a harmless method outside the CRS allowed-method policy (rule 911100).
func TestWAFProxyModesWithRealNginx(t *testing.T) {
	bin := os.Getenv("NGINX_TEST_BIN")
	if bin == "" {
		t.Skip("set NGINX_TEST_BIN to exercise the real Nginx proxy")
	}
	root, err := os.MkdirTemp("/tmp", "ngw-waf-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	for _, dir := range []string{"logs", "temp/body", "temp/proxy", "temp/fastcgi", "temp/scgi", "temp/uwsgi"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	manager := New(Paths{NginxRunDir: root})
	inspector := &waf.Inspector{}
	socket, err := net.Listen("unix", manager.paths.WAFSocket())
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: inspector}
	go server.Serve(socket)
	defer server.Close()
	var received atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%s %s %s", r.Method, r.RequestURI, body)
	}))
	defer backend.Close()
	upstream, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(upstream.Port())
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listen := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	var sites strings.Builder
	for i, mode := range []string{"detect", "block", "off"} {
		id := fmt.Sprintf("%016x", i+1)
		r := domain.ProxyRule{ID: id, Enabled: true, Name: mode, Domains: []string{mode + ".example.net"}, ListenPort: listen, UpstreamScheme: "http", UpstreamHost: upstream.Hostname(), UpstreamPort: port}
		domain.NormalizeRule(&r, domain.DefaultState().Settings)
		var instances []domain.WAFInstance
		if mode != "off" {
			instances = []domain.WAFInstance{{ID: id, Mode: mode, Sites: []string{id}}}
		}
		site, err := manager.renderRuleServer(r, nil, nil, instances...)
		if err != nil {
			t.Fatal(err)
		}
		sites.WriteString(site)
	}
	conf := fmt.Sprintf(`daemon off; master_process off; pid %s/pid; error_log stderr; events {} http {
 access_log off; map $host $fnproxy_rule_id {default default;} map $request_id $fnproxy_waf_checked {default 0;}
 map $http_host $fnproxy_client_host {default $http_host;} map $http_upgrade $connection_upgrade {default upgrade; '' close;}
 %s }`, root, sites.String())
	file := filepath.Join(root, "nginx.conf")
	if err = os.WriteFile(file, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(root, "nginx.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.Command(bin, "-p", root+"/", "-c", file)
	cmd.Stderr = logFile
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Signal(os.Interrupt); cmd.Wait() }()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listen))
	ready := false
	for range 100 {
		c, e := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		b, _ := os.ReadFile(logFile.Name())
		t.Fatalf("nginx startup: %s", b)
	}
	client := http.Client{Timeout: 5 * time.Second}
	for _, tc := range []struct {
		mode, method, contentType, body string
		want                            int
	}{
		{"detect", "BREW", "text/plain", "hello", 200}, {"block", "BREW", "text/plain", "hello", 403},
		{"off", "BREW", "text/plain", "hello", 200}, {"block", "POST", "application/json", `{"title":"hello"}`, 200},
		{"detect", "POST", "application/json", `{"title":`, 200}, {"block", "POST", "application/json", `{"title":`, 400},
	} {
		before := received.Load()
		uri := "/a%2Fb?q=two"
		body := tc.body
		req, _ := http.NewRequest(tc.method, "http://"+addr+uri, strings.NewReader(body))
		req.Host = tc.mode + ".example.net"
		req.Header.Set("Content-Type", tc.contentType)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s %s status=%d body=%s", tc.mode, tc.method, resp.StatusCode, data)
		}
		if tc.want == 200 {
			if string(data) != tc.method+" "+uri+" "+body || received.Load() != before+1 {
				t.Fatalf("upstream request changed: %s", data)
			}
		} else if received.Load() != before {
			t.Fatal("blocked request reached upstream")
		}
	}
	found := map[bool]bool{}
	for _, e := range inspector.Events() {
		for _, id := range e.Rules {
			if id == 911100 {
				found[e.Blocked] = true
			}
		}
	}
	if !found[true] || !found[false] {
		t.Fatalf("missing mode-specific events: %+v", inspector.Events())
	}
}
