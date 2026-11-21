package metrics

import "time"

// TopologyHit is deliberately kept out of the long-term metrics checkpoint.
type TopologyHit struct {
	Second                          int64
	Rule, Client, Upstream, Outcome string
	Server, Listener, Address       string
	Bytes                           uint64
}

type TopologyFlow struct {
	Server, Listener, Address string `json:"-"`
	Rule                      string `json:"rule"`
	Client                    string `json:"client"`
	Upstream                  string `json:"upstream"`
	Outcome                   string `json:"outcome,omitempty"`
	Requests                  int    `json:"requests"`
	Bytes                     uint64 `json:"bytes"`
}

func (c *Collector) Topology(now time.Time) ([]TopologyFlow, bool, *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	start := now.Unix() - 59
	type key struct{ rule, client, upstream, outcome, server, listener, address string }
	flows := map[key]*TopologyFlow{}
	for _, hit := range c.topology {
		if hit.Second < start || hit.Second > now.Unix() {
			continue
		}
		k := key{hit.Rule, hit.Client, hit.Upstream, hit.Outcome, hit.Server, hit.Listener, hit.Address}
		flow := flows[k]
		if flow == nil {
			flow = &TopologyFlow{Rule: k.rule, Client: k.client, Upstream: k.upstream, Outcome: k.outcome, Server: k.server, Listener: k.listener, Address: k.address}
			flows[k] = flow
		}
		flow.Requests++
		flow.Bytes += hit.Bytes
	}
	out := make([]TopologyFlow, 0, len(flows))
	for _, flow := range flows {
		out = append(out, *flow)
	}
	var connections *int
	if c.latest != nil && c.sampledAt != nil && now.Sub(*c.sampledAt) < 15*time.Second {
		n := c.latest.Connections
		connections = &n
	}
	return out, c.topology[c.topologyNext].Second >= start, connections
}

func (c *Collector) AddTopology(hit TopologyHit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topology[c.topologyNext] = hit
	c.topologyNext = (c.topologyNext + 1) % len(c.topology)
}
