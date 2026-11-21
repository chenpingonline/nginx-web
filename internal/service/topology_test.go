package service

import (
	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/metrics"
	"slices"
	"testing"
)

func TestTopologyDestination(t *testing.T) {
	const waf = "/run/nginx/waf.sock"
	for _, tc := range []struct{ raw, upstream, outcome string }{
		{"unix:" + waf, "", "waf_only"},
		{"unix:" + waf + " : 127.0.0.1:8418", "127.0.0.1:8418", ""},
		{"unix:" + waf + " : [::1]:80, 127.0.0.1:8418", "[::1]:80, 127.0.0.1:8418", ""},
		{"unix:/other/waf.sock", "unix:/other/waf.sock", ""},
		{"-", "", "no_upstream"},
		{"", "", "no_upstream"},
	} {
		upstream, outcome := topologyDestination(tc.raw, waf)
		if upstream != tc.upstream || outcome != tc.outcome {
			t.Errorf("%q: got (%q, %q), want (%q, %q)", tc.raw, upstream, outcome, tc.upstream, tc.outcome)
		}
	}
}

func TestCustomTopologySharedBackendAndCopiedID(t *testing.T) {
	cache := topologyCustomCache{}
	file := domain.CustomConfig{Name: "custom-dns.conf", Content: `server { listen 9090 ssl; listen [::]:9090 ssl; server_name "dns11.nascp.cn"; set $fnproxy_rule_id "managed"; location / { proxy_pass http://127.0.0.1:10053; } }`}
	sites := cache.load([]domain.CustomConfig{file, {Name: "custom-ignore.stream", Content: file.Content}})
	if len(sites) != 1 || sites[0].Listener != "HTTPS :9090" || !slices.Equal(sites[0].Targets, []string{"127.0.0.1:10053"}) {
		t.Fatal(sites)
	}
	v := topologyView{Flows: []metrics.TopologyFlow{
		{Rule: "managed", Server: "dns.nascp.cn", Listener: "HTTPS :9090", Address: "127.0.0.1"},
		{Rule: "managed", Server: "dns11.nascp.cn", Listener: "HTTPS :9090", Address: "127.0.0.1"},
		{Rule: "default", Server: "dns11.nascp.cn", Listener: "HTTPS :9090", Address: "::1"},
		{Rule: "default", Server: "unknown", Listener: "HTTPS :9090", Address: "127.0.0.1"},
	}}
	attachCustomTopology(&v, sites)
	if len(v.Flows) != 3 || v.Flows[0].Rule != "managed" || v.Flows[1].Rule != sites[0].ID || v.Flows[2].Rule != sites[0].ID {
		t.Fatal(v.Flows)
	}
	if got := cache.load(nil); len(got) != 0 {
		t.Fatal(got)
	}
}
