package metrics

import (
	"testing"
	"time"
)

func TestTopologyWindowAndCapacity(t *testing.T) {
	var c Collector
	now := time.Unix(1800000000, 0)
	hit := TopologyHit{Second: now.Unix() - 60, Rule: "site", Client: "192.0.2.10", Upstream: "192.0.2.1:80", Bytes: 42}
	c.AddTopology(hit)
	hit.Second++
	c.AddTopology(hit)
	hit.Second = now.Unix()
	c.AddTopology(hit)
	flows, truncated, _ := c.Topology(now)
	if truncated || len(flows) != 1 || flows[0].Bytes != 84 || flows[0].Requests != 2 {
		t.Fatalf("unexpected window: %+v", flows)
	}
	for range len(c.topology) {
		c.AddTopology(hit)
	}
	flows, truncated, _ = c.Topology(now)
	if !truncated || len(flows) != 1 || flows[0].Requests != len(c.topology) {
		t.Fatal("saturated window should report limited sample")
	}
	flows, truncated, _ = c.Topology(now.Add(time.Minute))
	if len(flows) != 0 || truncated {
		t.Fatal("expired traffic must disappear")
	}
}

func TestTopologyKeepsWAFAndLocalResponsesSeparate(t *testing.T) {
	var c Collector
	now := time.Now()
	hit := TopologyHit{Second: now.Unix(), Rule: "git", Client: "127.0.0.1", Bytes: 574, Outcome: "waf_only"}
	for range 4 {
		c.AddTopology(hit)
	}
	hit.Outcome = "no_upstream"
	c.AddTopology(hit)
	flows, _, _ := c.Topology(now)
	if len(flows) != 2 {
		t.Fatalf("got %d flows, want 2", len(flows))
	}
	for _, flow := range flows {
		switch flow.Outcome {
		case "waf_only":
			if flow.Requests != 4 || flow.Bytes != 2296 {
				t.Fatalf("unexpected WAF flow: %+v", flow)
			}
		case "no_upstream":
			if flow.Requests != 1 || flow.Bytes != 574 {
				t.Fatalf("unexpected local flow: %+v", flow)
			}
		default:
			t.Fatalf("unexpected outcome: %+v", flow)
		}
	}
}
