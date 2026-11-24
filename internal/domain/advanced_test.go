package domain

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestNormalizeStreamRuleSerializesEmptyListsAsArrays(t *testing.T) {
	for _, input := range []string{
		`{"name":"legacy","protocol":"tcp"}`,
		`{"name":"legacy","protocol":"udp","trusted_proxies":null,"allow":null,"deny":null,"sni_routes":null}`,
	} {
		var rule StreamRule
		if err := json.Unmarshal([]byte(input), &rule); err != nil {
			t.Fatal(err)
		}
		NormalizeStreamRule(&rule)
		encoded, err := json.Marshal(rule)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"trusted_proxies", "allow", "deny", "sni_routes"} {
			if string(fields[field]) != "[]" {
				t.Errorf("%s must be an empty JSON array, got %s", field, fields[field])
			}
		}
	}
}

func TestNormalizeStreamRulePreservesListsAndInitializesRouteNames(t *testing.T) {
	rule := StreamRule{
		TrustedProxies: []string{"127.0.0.1"}, Allow: []string{"10.0.0.0/8"}, Deny: []string{"all"},
		SNIRoutes: []SNIRoute{{ServerNames: []string{" EXAMPLE.TEST "}, UpstreamHost: "127.0.0.1", UpstreamPort: 443}, {}},
	}
	NormalizeStreamRule(&rule)
	if len(rule.TrustedProxies) != 1 || rule.TrustedProxies[0] != "127.0.0.1" || len(rule.Allow) != 1 || rule.Allow[0] != "10.0.0.0/8" || len(rule.Deny) != 1 || rule.Deny[0] != "all" {
		t.Fatal("normalization changed configured access lists")
	}
	if len(rule.SNIRoutes) != 2 || rule.SNIRoutes[0].ServerNames[0] != "example.test" || rule.SNIRoutes[0].UpstreamPort != 443 || rule.SNIRoutes[1].ServerNames == nil {
		t.Fatalf("unexpected normalized SNI routes: %+v", rule.SNIRoutes)
	}
}

func TestValidateStateAcceptsAdvancedRuntimeAndUpstreamPool(t *testing.T) {
	state := DefaultState()
	state.Settings.RealIP = RealIPSettings{Enabled: true, Header: "X-Forwarded-For", TrustedProxies: []string{"10.0.0.0/8"}, Recursive: true}
	pool := UpstreamPool{ID: "0123456789ab", Name: "web", Protocol: "http", Strategy: "least_conn", Keepalive: 32, Servers: []UpstreamServer{{Host: "127.0.0.1", Port: 8080}}}
	NormalizeUpstreamPool(&pool)
	pool.CreatedAt = time.Now().UTC()
	pool.UpdatedAt = pool.CreatedAt
	state.UpstreamPools = []UpstreamPool{pool}
	rule := ProxyRule{ID: "abcdef012345", Name: "demo", Enabled: true, ListenPort: 19080, Domains: []string{"demo.test"}, UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, UpstreamPoolID: pool.ID, ConnectTimeoutSeconds: 10, ReadTimeoutSeconds: 60, SendTimeoutSeconds: 60, RateLimit: RateLimitSettings{Enabled: true, RequestsPerSecond: 20, Burst: 40, Connections: 10}}
	state.Rules = []ProxyRule{rule}
	if err := ValidateState(state); err != nil {
		t.Fatal(err)
	}
}

func TestValidateStateRejectsUntrustedRealIPAndMissingPool(t *testing.T) {
	state := DefaultState()
	state.Settings.RealIP.Enabled = true
	if err := ValidateState(state); err == nil {
		t.Fatal("expected Real IP validation error")
	}
	state = DefaultState()
	rule := ProxyRule{ID: "abcdef012345", Name: "demo", Enabled: true, ListenPort: 19080, Domains: []string{"demo.test"}, UpstreamScheme: "http", UpstreamPoolID: "0123456789ab", ConnectTimeoutSeconds: 10, ReadTimeoutSeconds: 60, SendTimeoutSeconds: 60}
	state.Rules = []ProxyRule{rule}
	if err := ValidateState(state); err == nil {
		t.Fatal("expected missing pool validation error")
	}
}

func TestAccessAllIsNotATrustedProxyNetwork(t *testing.T) {
	if err := validateAccessAddresses([]string{"127.0.0.1", "10.0.0.0/8", "all"}); err != nil {
		t.Fatal(err)
	}
	if validateAccessAddresses([]string{"any"}) == nil || validateCIDRs([]string{"all"}) == nil {
		t.Fatal("only access rules accept the literal all")
	}
}

func TestNormalizeLegacyStaticTryFiles(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"$uri $uri/ /index.html"}, []string{"$uri", "$uri/", "/index.html"}},
		{[]string{"/file with spaces", "=404"}, []string{"/file with spaces", "=404"}},
	} {
		loc := LocationSettings{BackendType: "static", TryFiles: tc.input}
		NormalizeLocationSettings(&loc)
		if !slices.Equal(loc.TryFiles, tc.want) {
			t.Fatalf("got %q, want %q", loc.TryFiles, tc.want)
		}
	}
}

func TestDynamicRoutingAcceptsEmptyOutputs(t *testing.T) {
	settings := DynamicRoutingSettings{
		Maps:   []MapDefinition{{Name: "host", Source: "$host", Variable: "$backend", Entries: []KeyValue{{Key: "a.test"}}}},
		Geos:   []GeoDefinition{{Name: "area", Variable: "$area", Entries: []KeyValue{{Key: "127.0.0.1"}}}},
		Splits: []SplitDefinition{{Name: "variant", Source: "$request_id", Variable: "$variant", Entries: []KeyValue{{Key: "*"}}}},
	}
	if err := validateDynamicRouting(settings); err != nil {
		t.Fatal(err)
	}
	settings.Maps[0].Entries[0].Key = ""
	if err := validateDynamicRouting(settings); err == nil {
		t.Fatal("empty map key accepted")
	}
}

func TestDynamicRoutingDefaultsPreservePartialSettings(t *testing.T) {
	for _, existing := range []DynamicRoutingSettings{
		{Maps: []MapDefinition{{Variable: "$backend"}}},
		{Geos: []GeoDefinition{{Variable: "$area"}}, Splits: []SplitDefinition{{Variable: "$variant"}}},
	} {
		state := DefaultState()
		state.Settings.Routing = existing
		ApplyStateDefaults(&state)
		got := state.Settings.Routing
		if got.Maps == nil || got.Geos == nil || got.Splits == nil {
			t.Fatal("defaults left a missing routing list")
		}
		if len(got.Maps) != len(existing.Maps) || len(got.Geos) != len(existing.Geos) || len(got.Splits) != len(existing.Splits) {
			t.Fatal("defaults discarded existing routing blocks")
		}
	}
}

func TestProxyRedirectPoolBinding(t *testing.T) {
	id := "aaaaaaaaaaaa"
	pool := UpstreamPool{ID: id, Name: "Original", Protocol: "http"}
	settings := LocationSettings{BackendType: "proxy", UpstreamPoolID: id, ProxyRedirect: &ProxyRedirect{PoolID: id, OriginalHost: "Old_Pool"}}
	NormalizeLocationSettings(&settings)
	if settings.ProxyRedirect == nil {
		t.Fatal("lost original upstream")
	}
	pools := map[string]UpstreamPool{id: pool}
	if err := ValidateLocationSettings(settings, pools); err != nil {
		t.Fatal(err)
	}
	invalid := settings
	invalid.ProxyRedirect = &ProxyRedirect{PoolID: id, OriginalHost: "old/path"}
	if ValidateLocationSettings(invalid, pools) == nil {
		t.Fatal("accepted redirect source containing a path")
	}
	settings.UpstreamPoolID = "bbbbbbbbbbbb"
	NormalizeLocationSettings(&settings)
	if settings.ProxyRedirect != nil {
		t.Fatal("retained redirect from another pool")
	}
}
