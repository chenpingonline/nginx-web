package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/chenpingonline/nginx-web/internal/domain"

	crossplane "github.com/nginxinc/nginx-go-crossplane"
)

// Reuse parsed source files between dashboard and topology polls.
type customSiteCache struct {
	mu    sync.Mutex
	files []domain.CustomConfig
	sites []customSite
}
type customSite struct {
	topologySite
	file, protocol, port, server string
	names                        []string
	streamMetricsLog             string
	addresses                    []string
}

func (c *customSiteCache) load(files []domain.CustomConfig) []customSite {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !slices.Equal(c.files, files) {
		var sites []customSite
		for _, f := range files {
			sites = append(sites, parseCustomSites(f)...)
		}
		c.files = slices.Clone(files)
		c.sites = sites
	}
	return c.sites
}
func customListener(args []string, stream bool) (protocol, port, address string) {
	if len(args) == 0 {
		return
	}
	protocol = "HTTP"
	if stream {
		protocol = "TCP"
		if slices.Contains(args[1:], "udp") {
			protocol = "UDP"
		}
	} else if slices.Contains(args[1:], "ssl") || slices.Contains(args[1:], "quic") {
		protocol = "HTTPS"
	}
	address = args[0]
	if strings.HasPrefix(address, "unix:") {
		return
	}
	host, p, err := net.SplitHostPort(address)
	if err == nil {
		address, port = host, p
	} else if _, err := strconv.Atoi(address); err == nil {
		port, address = address, "0.0.0.0"
	} else if !stream {
		address, port = strings.Trim(address, "[]"), "80"
	}
	if address == "*" || address == "" {
		address = "0.0.0.0"
	}
	if ip := net.ParseIP(address); ip != nil {
		address = ip.String()
	}
	return
}
func topologyPassTargets(nodes crossplane.Directives, targets map[string]bool) {
	for _, d := range nodes {
		switch d.Directive {
		case "location", "if", "limit_except":
			topologyPassTargets(d.Block, targets)
		case "proxy_pass", "grpc_pass", "fastcgi_pass", "uwsgi_pass", "scgi_pass", "memcached_pass":
			if len(d.Args) != 1 {
				continue
			}
			target := d.Args[0]
			if strings.Contains(target, "://") {
				u, err := url.Parse(target)
				if err != nil || u.Host == "" {
					continue
				}
				target = u.Host
			}
			targets[target] = true
		}
	}
}
func parseCustomSites(f domain.CustomConfig) []customSite {
	p, err := crossplane.Parse(f.Name, &crossplane.ParseOptions{SingleFile: true, SkipDirectiveContextCheck: true, SkipDirectiveArgsCheck: true, Open: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(f.Content)), nil }})
	if err != nil || len(p.Config) != 1 || len(p.Errors) != 0 {
		return nil
	}
	var sites []customSite
	stream := strings.HasSuffix(f.Name, ".stream")
	type binding struct{ protocol, port string }
	seen := map[string]int{}
	for _, block := range p.Config[0].Parsed {
		if block.Directive != "server" {
			continue
		}
		var names []string
		listeners := map[binding][]string{}
		metricsLog := ""
		hasListen := false
		for _, d := range block.Block {
			switch d.Directive {
			case "listen":
				hasListen = true
				if protocol, port, host := customListener(d.Args, stream); protocol != "" {
					k := binding{protocol, port}
					listeners[k] = append(listeners[k], host)
				}
			case "access_log":
				if len(d.Args) >= 2 && strings.HasPrefix(d.Args[1], "fnproxy_stream_metrics_") {
					metricsLog = d.Args[0]
				}
			case "server_name":
				names = append(names, d.Args...)
			}
		}
		if !hasListen && !stream {
			port := "80"
			if os.Geteuid() != 0 {
				port = "8000"
			}
			listeners[binding{"HTTP", port}] = []string{"0.0.0.0"}
		}
		targets := map[string]bool{}
		topologyPassTargets(block.Block, targets)
		var sorted []string
		for t := range targets {
			sorted = append(sorted, t)
		}
		slices.Sort(sorted)
		label := strings.Join(names, ", ")
		server := ""
		if len(names) > 0 {
			server = names[0]
		}
		if label == "" {
			label = f.Name
		}
		for binding, addrs := range listeners {
			slices.Sort(addrs)
			addrs = slices.Compact(addrs)
			listener := binding.protocol + " :" + binding.port
			if binding.port == "" {
				listener = binding.protocol + " " + strings.Join(addrs, ", ")
			}
			identity := strings.Join([]string{f.Name, strings.ToLower(server), listener, strings.Join(addrs, ",")}, "\x00")
			seen[identity]++
			sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", identity, seen[identity])))
			sites = append(sites, customSite{
				topologySite: topologySite{ID: "custom-" + hex.EncodeToString(sum[:16]), Name: label, Listener: listener, Targets: sorted},
				file:         f.Name, protocol: binding.protocol, port: binding.port,
				server: server, names: names, addresses: addrs, streamMetricsLog: metricsLog,
			})
		}
	}
	return sites
}

// The server selected by Nginx is authoritative: custom blocks can carry a
// copied managed rule ID, and distinct sites can share the same backend.
type siteKey struct{ server, listener, address string }
type siteIndex map[siteKey]string

func (ids siteIndex) add(k siteKey, id string) {
	// An ambiguous binding cannot identify a server.
	if old, ok := ids[k]; ok && old != id {
		id = ""
	}
	ids[k] = id
}
func indexCustomSites(custom []customSite) siteIndex {
	ids := siteIndex{}
	for _, site := range custom {
		name := strings.ToLower(site.server)
		if site.protocol == "TCP" || site.protocol == "UDP" {
			name = ""
		}
		listener := site.protocol + " :" + site.port
		for _, addr := range site.addresses {
			ids.add(siteKey{name, listener, addr}, site.ID)
		}
	}
	return ids
}
func (ids siteIndex) lookup(server, listener, address string) string {
	if len(ids) == 0 || address == "" {
		return ""
	}
	k := siteKey{strings.ToLower(server), listener, address}
	if id, ok := ids[k]; ok {
		return id
	}
	k.address = "0.0.0.0"
	if strings.Contains(address, ":") {
		k.address = "::"
	}
	return ids[k]
}
func attachCustomTopology(v *topologyView, custom []customSite) {
	ids := indexCustomSites(custom)
	for _, site := range custom {
		if site.protocol == "HTTP" || site.protocol == "HTTPS" {
			v.Sites = append(v.Sites, site.topologySite)
		}
	}
	visible := v.Flows[:0]
	for _, flow := range v.Flows {
		if id := ids.lookup(flow.Server, flow.Listener, flow.Address); id != "" {
			flow.Rule = id
		}
		if flow.Rule != "" && flow.Rule != "default" {
			visible = append(visible, flow)
		}
	}
	v.Flows = visible
}

func (s *AppService) setCustomMetricRules(state State) {
	effective := domain.MenuRuntimeState(state)
	ids := indexCustomSites(s.appliedSites.load(effective.CustomConfigs))
	// A managed server and a custom server may have the same primary name and
	// binding. Preserve Nginx's logged rule in cases the endpoint cannot resolve.
	for _, rule := range effective.Rules {
		if !rule.Enabled || len(rule.Domains) == 0 {
			continue
		}
		name := strings.ToLower(rule.Domains[0])
		if name == "*" {
			name = "_"
		}
		protocol := "HTTP"
		if rule.TLS {
			protocol = "HTTPS"
		}
		for _, family := range domain.ListenFamilies(rule.ListenType) {
			addr := "0.0.0.0"
			if family == "ipv6" {
				addr = "::"
			}
			ids.add(siteKey{name, fmt.Sprintf("%s :%d", protocol, rule.ListenPort), addr}, "")
		}
	}
	s.metrics.SetCustomRuleLookup(ids.lookup)
	s.streamMetrics.SetCustomRuleLookup(ids.lookup)
}
