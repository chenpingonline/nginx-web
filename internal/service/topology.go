package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/metrics"
)

type topologySite struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Listener string   `json:"listener"`
	Targets  []string `json:"targets"`
}
type topologyView struct {
	Sites       []topologySite         `json:"sites"`
	Flows       []metrics.TopologyFlow `json:"flows"`
	Connections *int                   `json:"connections"`
	Limited     bool                   `json:"limited"`
	Logging     bool                   `json:"logging"`
	Time        int64                  `json:"time"`
}

func (s *AppService) Topology() topologyView {
	now := time.Now()
	state, _ := s.appliedState(s.store.Snapshot())
	state = domain.MenuRuntimeState(state)
	flows, limited, connections := s.metrics.Topology(now)
	master, _ := os.ReadFile(s.paths.NginxMaster)
	logging := strings.Contains(string(master), " fnproxy_topology;")
	v := topologyView{Sites: []topologySite{}, Flows: flows, Connections: connections, Limited: limited, Time: now.Unix(), Logging: logging}
	pools := map[string]domain.UpstreamPool{}
	for _, p := range state.UpstreamPools {
		pools[p.ID] = p
	}
	for _, r := range state.Rules {
		if !r.Enabled {
			continue
		}
		protocol := "HTTP"
		if r.TLS {
			protocol = "HTTPS"
		}
		site := topologySite{ID: r.ID, Name: strings.Join(r.Domains, ", "), Listener: fmt.Sprintf("%s :%d", protocol, r.ListenPort)}
		seen := map[string]bool{}
		add := func(loc domain.LocationSettings) {
			switch {
			case loc.UpstreamPoolID != "":
				for _, u := range pools[loc.UpstreamPoolID].Servers {
					seen[net.JoinHostPort(u.Host, strconv.Itoa(u.Port))] = true
				}
			case loc.UpstreamHost != "":
				seen[net.JoinHostPort(loc.UpstreamHost, strconv.Itoa(loc.UpstreamPort))] = true
			case loc.BackendType != "":
				seen[loc.BackendType] = true
			}
		}
		if r.RedirectToHTTPS {
			seen["HTTPS 跳转"] = true
		} else {
			root := r.RootLocation
			if root.UpstreamHost == "" && root.UpstreamPoolID == "" && (root.BackendType == "" || root.BackendType == "proxy") {
				root.UpstreamHost = r.UpstreamHost
				root.UpstreamPort = r.UpstreamPort
				root.UpstreamPoolID = r.UpstreamPoolID
			}
			add(root)
			for _, loc := range r.Locations {
				if loc.Enabled {
					add(loc.Settings)
				}
			}
		}
		for t := range seen {
			site.Targets = append(site.Targets, t)
		}
		sort.Strings(site.Targets)
		v.Sites = append(v.Sites, site)
	}
	attachCustomTopology(&v, s.topologyCustom.load(state.CustomConfigs))
	return v
}

// The normal access log policy controls collection. Visitor addresses for this
// view travel over a private datagram socket and never enter metrics history.
func (s *AppService) MaintainTopology(ctx context.Context) {
	path := s.paths.TopologySocket()
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSocket == 0 {
		log.Printf("拓扑路径已被普通文件占用: %s", path)
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("拓扑套接字: %v", err)
		return
	}
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		log.Printf("拓扑套接字: %v", err)
		return
	}
	defer os.Remove(path)
	defer conn.Close()
	if err := os.Chmod(path, 0660); err != nil {
		log.Print(err)
		return
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	buf := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFromUnix(buf)
		if err != nil {
			return
		}
		i := bytes.IndexByte(buf[:n], '{')
		if i < 0 {
			continue
		}
		var record struct {
			metrics.TopologyHit
			UpstreamAddress *string `json:"upstream"`
			Scheme          string  `json:"scheme"`
			Port            string  `json:"port"`
		}
		if json.Unmarshal(buf[i:n], &record) != nil || ((record.Rule == "default" || record.Rule == "") && record.Port == "") {
			continue
		}
		hit := record.TopologyHit
		if record.Port != "" {
			hit.Listener = strings.ToUpper(record.Scheme) + " :" + record.Port
		}
		if record.UpstreamAddress != nil {
			hit.Upstream, hit.Outcome = topologyDestination(*record.UpstreamAddress, s.paths.WAFSocket())
		}
		hit.Second = time.Now().Unix()
		s.metrics.AddTopology(hit)
	}
}

// Only the final group handles the outcome after an internal redirect.
// Within it, Nginx lists retry attempts in order.
func topologyDestination(raw, wafSocket string) (string, string) {
	group := raw
	if i := strings.LastIndex(raw, " : "); i >= 0 {
		group = raw[i+3:]
	}
	var attempts []string
	outcome := "no_upstream"
	for _, addr := range strings.Split(group, ",") {
		switch addr = strings.TrimSpace(addr); addr {
		case "", "-":
			continue
		case "unix:" + wafSocket:
			outcome = "waf_only"
		default:
			attempts = append(attempts, addr)
		}
	}
	if len(attempts) > 0 {
		return strings.Join(attempts, ", "), ""
	}
	return "", outcome
}
