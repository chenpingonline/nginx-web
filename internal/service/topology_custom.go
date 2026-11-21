package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/chenpingonline/nginx-web/internal/domain"

	crossplane "github.com/nginxinc/nginx-go-crossplane"
)

// Reuse the parsed configuration between polls. Only applied HTTP files participate.
type topologyCustomCache struct {
	mu    sync.Mutex
	files []domain.CustomConfig
	sites []customTopologySite
}
type customTopologySite struct {
	topologySite
	server    string
	addresses []string
}

func (c *topologyCustomCache) load(files []domain.CustomConfig) []customTopologySite {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !slices.Equal(c.files, files) {
		var sites []customTopologySite
		for _, f := range files {
			if strings.HasSuffix(f.Name, ".conf") {
				sites = append(sites, parseTopologySites(f)...)
			}
		}
		c.files = slices.Clone(files)
		c.sites = sites
	}
	return c.sites
}
func topologyListener(args []string) (string, string) {
	if len(args) == 0 {
		return "", ""
	}
	address := args[0]
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if net.ParseIP(strings.Trim(address, "[]")) != nil {
			host = strings.Trim(address, "[]")
			port = "80"
		} else {
			host = "*"
			port = address
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", ""
	}
	proto := "HTTP"
	if slices.Contains(args[1:], "ssl") {
		proto = "HTTPS"
	}
	return fmt.Sprintf("%s :%d", proto, n), host
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
func parseTopologySites(f domain.CustomConfig) []customTopologySite {
	p, err := crossplane.Parse(f.Name, &crossplane.ParseOptions{SingleFile: true, SkipDirectiveContextCheck: true, SkipDirectiveArgsCheck: true, Open: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(f.Content)), nil }})
	if err != nil || len(p.Config) != 1 {
		return nil
	}
	var sites []customTopologySite
	for index, block := range p.Config[0].Parsed {
		if block.Directive != "server" {
			continue
		}
		var names []string
		listeners := map[string][]string{}
		hasListen := false
		for _, d := range block.Block {
			switch d.Directive {
			case "listen":
				hasListen = true
				if label, host := topologyListener(d.Args); label != "" {
					listeners[label] = append(listeners[label], host)
				}
			case "server_name":
				names = append(names, d.Args...)
			}
		}
		if !hasListen {
			listeners["HTTP :80"] = []string{"*"}
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
		for _, listener := range slices.Sorted(maps.Keys(listeners)) {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", f.Name, index, listener)))
			sites = append(sites, customTopologySite{topologySite: topologySite{ID: "custom-" + hex.EncodeToString(sum[:16]), Name: label, Listener: listener, Targets: sorted}, server: server, addresses: listeners[listener]})
		}
	}
	return sites
}

// The server selected by Nginx is authoritative: custom blocks can carry a
// copied managed rule ID, and distinct sites can share the same backend.
func attachCustomTopology(v *topologyView, custom []customTopologySite) {
	type key struct{ server, listener, address string }
	ids := map[key]string{}
	for _, site := range custom {
		v.Sites = append(v.Sites, site.topologySite)
		for _, addr := range site.addresses {
			if addr == "*" {
				addr = "0.0.0.0"
			}
			ids[key{strings.ToLower(site.server), site.Listener, addr}] = site.ID
		}
	}
	visible := v.Flows[:0]
	for _, flow := range v.Flows {
		k := key{strings.ToLower(flow.Server), flow.Listener, flow.Address}
		id := ids[k]
		if id == "" && flow.Address != "" {
			k.address = "0.0.0.0"
			if strings.Contains(flow.Address, ":") {
				k.address = "::"
			}
			id = ids[k]
		}
		if id != "" {
			flow.Rule = id
		}
		if flow.Rule != "" && flow.Rule != "default" {
			visible = append(visible, flow)
		}
	}
	v.Flows = visible
}
