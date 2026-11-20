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
