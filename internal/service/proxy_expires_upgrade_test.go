package service

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestLegacyProxyExpiresVersion(t *testing.T) {
	for _, v := range []string{"0.0.1", "v0.2.100", "0.3.9", "0.3.32"} {
		if !legacyProxyExpiresVersion(v) {
			t.Errorf("missed legacy version %q", v)
		}
	}
	for _, v := range []string{"", "0.3", "0.3.33", "0.3.62", "v0.4.0", "1.0.0", "garbage", "0.3.032", "0.3.32-beta", "0.3.32+custom"} {
		if legacyProxyExpiresVersion(v) {
			t.Errorf("unexpected migration for %q", v)
		}
	}
}

func TestProxyExpiresUpgrade(t *testing.T) {
	for _, tc := range []struct {
		version string
		legacy  bool
	}{{"0.3.32", true}, {"0.3.62", false}, {"", false}} {
		t.Run("from-"+tc.version, func(t *testing.T) {
			s := testService(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "private, no-store")
				fmt.Fprint(w, "upstream")
			}))
			defer upstream.Close()
			l, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := l.Addr().(*net.TCPAddr).Port
			l.Close()
			files := t.TempDir()
			t.Setenv("TRIM_DATA_ACCESSIBLE_PATHS", files)
			if err := os.WriteFile(filepath.Join(files, "item"), []byte("static"), 0644); err != nil {
				t.Fatal(err)
			}
			proxy := domain.LocationSettings{BackendType: "proxy", UpstreamHost: "127.0.0.1", UpstreamPort: upstream.Listener.Addr().(*net.TCPAddr).Port, UpstreamScheme: "http", Expires: "7d"}
			static := domain.LocationSettings{BackendType: "static", StaticAlias: true, StaticPath: files + "/", Expires: "1h"}
			base := domain.DefaultState()
			now := time.Now().UTC()
			base.LastAppliedAt = &now
			base.RuntimeConfigVersion = runtimeConfigVersion
			base.Rules = []domain.ProxyRule{{ID: "abcdef123456", Name: "cache", Domains: []string{"cache.test"}, Enabled: true, ListenPort: port, UpstreamHost: proxy.UpstreamHost, UpstreamPort: proxy.UpstreamPort, RootLocation: proxy, Locations: []domain.LocationRule{
				{ID: "abcdef123457", Name: "proxy", Enabled: true, Path: "/proxy/", Match: "prefix", Settings: proxy},
				{ID: "abcdef123458", Name: "static", Enabled: true, Path: "/static/", Match: "prefix", Settings: static},
				{ID: "abcdef123459", Name: "switch", Enabled: true, Path: "/switch/", Match: "prefix", Settings: static},
			}}}
			domain.ApplyStateDefaults(&base)
			draft := domain.CloneState(base)
			draft.Dirty = true
			draft.Rules[0].Locations[2].Settings = proxy
			draft.Rules[0].RootLocation.Expires = "2d"
			draft.Rules[0].Locations[0].Settings.Expires = "3d"
			draft.Rules[0].Locations[1].Settings.Expires = "2h"
			writeMigrationJSON(t, s.paths.StateFile, draft)
			writeMigrationJSON(t, s.paths.AppliedState(), base)
			s, err = New(s.paths)
			if err != nil {
				t.Fatal(err)
			}
			migrationNginx(t, s)
			if _, err := s.PrepareUpgrade(tc.version); err != nil {
				t.Fatal(err)
			}
			current := s.State()
			applied, ok := s.appliedState(current)
			if !ok || !current.Dirty {
				t.Fatal("lost applied baseline or draft")
			}
			check := func(st State, root, child, static string) {
				t.Helper()
				r := st.Rules[0]
				if tc.legacy {
					root, child = "", ""
				}
				if r.RootLocation.Expires != root || r.Locations[0].Settings.Expires != child || r.Locations[1].Settings.Expires != static {
					t.Errorf("unexpected root, child, static Expires: %q, %q, %q", r.RootLocation.Expires, r.Locations[0].Settings.Expires, r.Locations[1].Settings.Expires)
				}
			}
			check(current, "2d", "3d", "2h")
			check(applied, "7d", "7d", "1h")
			if os.Getenv("NGINX_TEST_BIN") != "" {
				if _, err := s.NginxStart(); err != nil {
					t.Fatal(err)
				}
				defer s.nginx.Stop()
				headers := func(path, want string, expires bool) {
					t.Helper()
					client := http.Client{Timeout: time.Second}
					var got string
					for i := 0; i < 60; i++ {
						req, _ := http.NewRequest("GET", "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
						req.Host = "cache.test"
						resp, e := client.Do(req)
						if e == nil {
							body, _ := io.ReadAll(resp.Body)
							resp.Body.Close()
							got = fmt.Sprintf("status=%d cache=%q expires=%q body=%.60q", resp.StatusCode, resp.Header.Get("Cache-Control"), resp.Header.Get("Expires"), body)
							if resp.StatusCode == 200 && resp.Header.Get("Cache-Control") == want && (resp.Header.Get("Expires") != "") == expires {
								return
							}
						}
						time.Sleep(25 * time.Millisecond)
					}
					t.Fatalf("%s want %q; got %s", path, want, got)
				}
				proxyHeaders := func(path, modern string) {
					if tc.legacy {
						headers(path, "private, no-store", false)
					} else {
						headers(path, modern, true)
					}
				}
				proxyHeaders("/", "max-age=604800")
				proxyHeaders("/proxy/item", "max-age=604800")
				headers("/static/item", "max-age=3600", true)
				headers("/switch/item", "max-age=3600", true)
				if _, err := s.Apply("apply pending edits"); err != nil {
					t.Fatal(err)
				}
				proxyHeaders("/", "max-age=172800")
				proxyHeaders("/proxy/item", "max-age=259200")
				headers("/static/item", "max-age=7200", true)
				proxyHeaders("/switch/item", "max-age=604800")
			}
			if tc.legacy {
				if err := s.updateState(func(st *State) error { st.Rules[0].RootLocation.Expires = "4d"; return nil }); err != nil {
					t.Fatal(err)
				}
				if _, err := s.PrepareUpgrade(tc.version); err != nil {
					t.Fatal(err)
				}
				if s.State().Rules[0].RootLocation.Expires != "4d" {
					t.Fatal("repeated upgrade erased explicit setting")
				}
			}
		})
	}
}
