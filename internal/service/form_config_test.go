package service

import (
	"encoding/json"
	"github.com/chenpingonline/nginx-web/internal/nginx"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func mustForm[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func formBody(s string) string {
	for strings.HasPrefix(s, formPrefix) {
		_, s, _ = strings.Cut(s, "\n")
	}
	return s
}

func TestGeneratedFormRoundTrips(t *testing.T) {
	s := testService(t)
	static := t.TempDir()
	t.Setenv("TRIM_DATA_ACCESSIBLE_PATHS", static)
	cert := domain.RandomID()
	if _, err := s.deployACME(cert, "tls", acmeResource(t, 73)); err != nil {
		t.Fatal(err)
	}
	group := mustForm(s.SaveRuleGroup("", domain.RuleGroup{Name: "入口", ListenPort: 19443, TLS: true, HTTP2: true, CertificateID: cert}))
	auth := mustForm(s.CreateAuthProfile(AuthProfileInput{Name: "users", Users: []AuthUserInput{{Username: "alice", Password: "test-pass-01", Enabled: true}}}))
	policy := mustForm(s.CreateRateLimitPolicy(RateLimitPolicy{Name: "limit", Settings: domain.RateLimitSettings{RequestsPerSecond: 9, Burst: 13, Connections: 2, NoDelay: true, DownloadKBps: 24}}))
	pool := mustForm(s.CreateUpstreamPool(UpstreamPool{Name: "HTTP nodes", Protocol: "http", Strategy: "least_conn", Keepalive: 8, Servers: []domain.UpstreamServer{{Host: "127.0.0.1", Port: 18001}, {Host: "::1", Port: 18002, Backup: true}}}))
	sp := mustForm(s.CreateUpstreamPool(UpstreamPool{Name: "TCP nodes", Protocol: "stream", Strategy: "round_robin", Servers: []domain.UpstreamServer{{Host: "::1", Port: 12345}}}))
	settings := s.State().Settings
	settings.Routing = domain.DynamicRoutingSettings{Maps: []domain.MapDefinition{{Name: "host", Source: "$host", Variable: "$mapped", Default: "off", Entries: []domain.KeyValue{{Key: "a.test", Value: "on"}}}}, Geos: []domain.GeoDefinition{{Name: "area", Variable: "$area", Default: "other", Entries: []domain.KeyValue{{Key: "127.0.0.1/32", Value: "local"}}}}, Splits: []domain.SplitDefinition{{Name: "canary", Source: "$request_id", Variable: "$variant", Entries: []domain.KeyValue{{Key: "10%", Value: "a"}, {Key: "*", Value: "b"}}}}}
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	r := ProxyRule{Name: "TLS site", Enabled: true, Domains: []string{"z.test", "a.test", "z.test"}, GroupID: group.ID, InheritFields: []string{"listen_port", "tls", "http2", "certificate_id"}, UpstreamHost: "127.0.0.1", UpstreamPort: 18001, UpstreamScheme: "https", WebSocket: true, Streaming: true, RateLimitPolicyID: policy.ID, Authentication: domain.RuleAuthentication{Enabled: true, Mode: "basic", ProfileID: auth.ID}, RootLocation: domain.LocationSettings{BackendType: "proxy", UpstreamScheme: "https", UpstreamPoolID: pool.ID}}
	for _, k := range []string{"proxy", "static", "return", "grpc", "fastcgi", "uwsgi", "scgi", "memcached", "status"} {
		loc := domain.LocationSettings{BackendType: k, UpstreamHost: "127.0.0.1", UpstreamPort: 9000, UpstreamScheme: "http"}
		switch k {
		case "static":
			loc.StaticAlias = true
			loc.StaticPath = static
			loc.AutoIndex = true
			loc.TryFiles = []string{"$uri", "=404"}
		case "return":
			loc.ReturnCode = 302
			loc.ReturnTarget = "/next"
		case "proxy":
			loc.ProxyPassTrailingSlash = true
			loc.Expires = "30m"
			loc.ResponseHeaders = []domain.HeaderSetting{{Name: "Cache-Control", Value: "public", Always: true}}
			loc.Cache = domain.CacheSettings{Enabled: true, KeysZoneMB: 5, MaxSizeMB: 32, InactiveMinutes: 5, ValidSeconds: 15}
		}
		r.Locations = append(r.Locations, domain.LocationRule{ID: domain.RandomID(), Name: k, Path: "/" + k + "/", Match: "prefix", Enabled: true, Settings: loc})
	}
	r.Locations = append(r.Locations, domain.LocationRule{ID: domain.RandomID(), Name: "disabled", Path: "/disabled/", Enabled: false, Settings: domain.LocationSettings{BackendType: "return", ReturnCode: 404}})
	r = mustForm(s.CreateRule(r))
	waf := mustForm(s.SaveWAF("", domain.WAFInstance{Name: "check", Mode: "detect", Sites: []string{r.ID}, BodyLimitMiB: 8, BodyLimitAction: "partial", UploadSites: []string{r.ID}}))
	tcp := mustForm(s.CreateStreamRule(StreamRule{Name: "SNI", Enabled: true, Protocol: "tcp", ListenPort: 19005, UpstreamPoolID: sp.ID, TLSMode: "passthrough", SNIRoutes: []domain.SNIRoute{{ServerNames: []string{"sni.test"}, UpstreamHost: "127.0.0.1", UpstreamPort: 12346}}, AccessLog: true, MaxConnections: 3}))
	udp := mustForm(s.CreateStreamRule(StreamRule{Name: "UDP", Enabled: true, Protocol: "udp", ListenPort: 19006, UpstreamHost: "::1", UpstreamPort: 5533, UDPResponses: 2}))
	for _, tc := range []struct {
		kind, id  string
		fragments []string
	}{
		{"rules", r.ID, []string{"server_name z.test a.test;", "http2 on;", "grpc_pass", "fastcgi_pass", "scgi_pass", "uwsgi_pass", "memcached_pass", "alias ", "return 302", "stub_status", "proxy_cache_valid", "auth_request", "fnproxy_checked"}},
		{"streams", tcp.ID, []string{"ssl_preread on", "proxy_pass $fnproxy_stream_backend_"}}, {"streams", udp.ID, []string{"udp reuseport", "proxy_responses 2"}},
		{"settings", "", []string{"map $host $mapped", "geo $area", "split_clients", "worker_connections"}},
		{"upstreams", pool.ID, []string{"least_conn", "backup", "keepalive 8"}}, {"upstreams", sp.ID, []string{"upstream fnproxy_stream_"}},
		{"rule-groups", group.ID, []string{"19443 ssl", "http2 on"}}, {"rate-limits", policy.ID, []string{"rate=9r/s", "burst=13 nodelay", "limit_rate 24k"}},
		{"auth-profiles", auth.ID, []string{"internal/basic-auth/"}}, {"waf", waf.ID, []string{"X-Fnproxy-Mode", "partial"}},
	} {
		t.Run(tc.kind+"/"+tc.id, func(t *testing.T) {
			copied := mustForm(s.ExportForm(tc.kind, tc.id))
			before := formBody(copied)
			for _, fragment := range tc.fragments {
				if !strings.Contains(before, fragment) {
					t.Fatalf("missing %q in generated directives", fragment)
				}
			}
			f := mustForm(s.ReadForm(strings.ReplaceAll(copied, "\n", "\r\n")))
			if f.Kind != tc.kind || f.ID != tc.id {
				t.Fatal("wrong form")
			}
			switch f.Kind {
			case "rules":
				var x ProxyRule
				decodeForm(t, f, &x)
				if len(x.Locations) != 10 || x.Locations[9].Enabled || x.Domains[0] != "z.test" {
					t.Fatal("lost ordered or unpublished settings")
				}
				mustForm(s.UpdateRule(x.ID, x))
			case "streams":
				var x StreamRule
				decodeForm(t, f, &x)
				mustForm(s.UpdateStreamRule(x.ID, x))
			case "settings":
				var x Settings
				decodeForm(t, f, &x)
				if err := s.UpdateSettings(x); err != nil {
					t.Fatal(err)
				}
			case "upstreams":
				var x UpstreamPool
				decodeForm(t, f, &x)
				mustForm(s.UpdateUpstreamPool(x.ID, x))
			case "rule-groups":
				var x domain.RuleGroup
				decodeForm(t, f, &x)
				mustForm(s.SaveRuleGroup(x.ID, x))
			case "rate-limits":
				var x RateLimitPolicy
				decodeForm(t, f, &x)
				mustForm(s.UpdateRateLimitPolicy(x.ID, x))
			case "waf":
				var x domain.WAFInstance
				decodeForm(t, f, &x)
				mustForm(s.SaveWAF(x.ID, x))
			case "auth-profiles":
				var x domain.AuthProfile
				decodeForm(t, f, &x)
				if x.Users[0].PasswordHash != "" {
					t.Fatal("exported authentication hash")
				}
				mustForm(s.UpdateAuthProfile(x.ID, AuthProfileInput{Name: x.Name, Realm: x.Realm, Users: []AuthUserInput{{ID: x.Users[0].ID, Username: x.Users[0].Username, Enabled: true}}}))
			}
			after := formBody(mustForm(s.ExportForm(tc.kind, tc.id)))
			if before != after {
				t.Fatal("configuration changed after saving form")
			}
			if _, err := s.ReadForm(copied + "\n# modified"); err == nil {
				t.Fatal("stale body accepted")
			}
		})
	}
	if bin := os.Getenv("NGINX_TEST_BIN"); bin != "" {
		paths := s.paths
		paths.NginxBin = bin
		if err := os.MkdirAll(filepath.Dir(paths.MimeTypes), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.MimeTypes, []byte("types { text/html html; }"), 0640); err != nil {
			t.Fatal(err)
		}
		if _, err := nginx.New(paths).TestState(s.State()); err != nil {
			t.Fatal(err)
		}
	}
}
func decodeForm(t *testing.T, f ConfigForm, v any) {
	t.Helper()
	if err := json.Unmarshal(f.Value, v); err != nil {
		t.Fatal(err)
	}
}

func TestImportFormIntoFreshStore(t *testing.T) {
	source := testService(t)
	p := mustForm(source.CreateUpstreamPool(UpstreamPool{Name: "original", Protocol: "http", Strategy: "round_robin", Servers: []domain.UpstreamServer{{Host: "127.0.0.1", Port: 19080}}}))
	copied := mustForm(source.ExportForm("upstreams", p.ID))
	target := testService(t)
	f := mustForm(target.ReadForm(copied))
	if len(target.State().UpstreamPools) != 0 {
		t.Fatal("preview wrote draft")
	}
	mustForm(target.CreateImportedForm(f))
	if _, err := target.CreateImportedForm(f); err == nil {
		t.Fatal("duplicate create succeeded")
	}
	if got := formBody(mustForm(target.ExportForm("upstreams", p.ID))); got != formBody(copied) {
		t.Fatal("pool behavior changed")
	}
}

func TestFormImportRelocatesManagedPaths(t *testing.T) {
	source, target := testService(t), testService(t)
	stream := mustForm(source.CreateStreamRule(StreamRule{Name: "TCP", Enabled: true, Protocol: "tcp", ListenPort: 19006, UpstreamHost: "127.0.0.1", UpstreamPort: 5533, AccessLog: true}))
	auth := mustForm(source.CreateAuthProfile(AuthProfileInput{Name: "users", Users: []AuthUserInput{{Username: "reader", Password: "test-reader-123", Enabled: true}}}))
	for _, tc := range []struct{ kind, id string }{{"settings", ""}, {"streams", stream.ID}, {"auth-profiles", auth.ID}} {
		t.Run(tc.kind, func(t *testing.T) {
			copied := mustForm(source.ExportForm(tc.kind, tc.id))
			f := mustForm(target.ReadForm(copied))
			if len(f.Paths) == 0 {
				t.Fatal("expected managed paths")
			}
			if f.Kind == "auth-profiles" {
				var profile domain.AuthProfile
				decodeForm(t, f, &profile)
				input := AuthProfileInput{Name: profile.Name, Users: []AuthUserInput{{Username: profile.Users[0].Username, Enabled: true}}}
				if _, err := target.CreateAuthProfileWithID(f.ID, input); err == nil {
					t.Fatal("created user without password")
				}
				input.Users[0].Password = "new-password-01"
				mustForm(target.CreateAuthProfileWithID(f.ID, input))
			}
		})
	}
}

func TestRejectChangedReferencedUpstream(t *testing.T) {
	s := testService(t)
	pool := mustForm(s.CreateUpstreamPool(UpstreamPool{Name: "pool", Protocol: "stream", Servers: []domain.UpstreamServer{{Host: "127.0.0.1", Port: 19001}}}))
	r := mustForm(s.CreateStreamRule(StreamRule{Name: "TCP", Enabled: true, Protocol: "tcp", ListenPort: 19006, UpstreamPoolID: pool.ID}))
	copied := mustForm(s.ExportForm("streams", r.ID))
	pool.Servers[0].Port++
	mustForm(s.UpdateUpstreamPool(pool.ID, pool))
	if _, err := s.ReadForm(copied); err == nil {
		t.Fatal("accepted a different upstream on the same ID")
	}
}
