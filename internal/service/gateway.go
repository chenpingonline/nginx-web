package service

import (
	"errors"
	"fmt"
	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/platform"
	"strings"
)

func (s *AppService) DiscoverGatewayApps() (platform.AppDiscovery, error) {
	root := s.paths.AppRegistryDir
	if root == "" {
		root = "/var/apps"
	}
	result, err := platform.DiscoverAppEntries(root)
	if err != nil {
		return result, err
	}
	decorateGatewayApps(&result, s.State().GatewayRoutes)
	return result, nil
}

func decorateGatewayApps(result *platform.AppDiscovery, routes []domain.GatewayRoute) {
	used := map[string]bool{}
	for _, route := range routes {
		used[route.Path] = true
	}
	for i := range result.Entries {
		entry := &result.Entries[i]
		if entry.GatewayURL != "" {
			continue
		}
		for _, route := range routes {
			if (entry.Target != "" && strings.TrimRight(route.Target, "/") == strings.TrimRight(entry.Target, "/")) ||
				(entry.Target == "" && route.Name != "" && ((entry.Name != "" && strings.EqualFold(route.Name, entry.Name)) || strings.EqualFold(route.Name, entry.App))) {
				entry.Existing = true
				entry.Target = route.Target
				entry.Path = route.Path
				entry.Note = "已存在于网关配置（含草稿），无需重复添加"
				break
			}
		}
		if entry.Existing {
			continue
		}
		var slug strings.Builder
		for _, r := range strings.ToLower(entry.App) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				slug.WriteRune(r)
			} else {
				slug.WriteByte('-')
			}
		}
		base := strings.Trim(slug.String(), "-")
		if base == "" {
			base = "app"
		}
		if len(base) > 40 {
			base = strings.TrimRight(base[:40], "-")
		}
		path := base
		for n := 2; used[path]; n++ {
			path = fmt.Sprintf("%s-%d", base, n)
		}
		entry.Path = path
		used[path] = true
	}
}

func (s *AppService) SaveGatewayRoute(id string, input domain.GatewayRoute) (domain.GatewayRoute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	domain.NormalizeGatewayRoute(&input)
	if id == "" {
		input.ID = domain.RandomID()
	} else {
		input.ID = id
	}
	err := s.updateState(func(state *State) error {
		if id == "" {
			state.GatewayRoutes = append(state.GatewayRoutes, input)
		} else {
			found := false
			for i := range state.GatewayRoutes {
				if state.GatewayRoutes[i].ID == id {
					state.GatewayRoutes[i] = input
					found = true
					break
				}
			}
			if !found {
				return errors.New("找不到指定网关规则")
			}
		}
		state.Dirty = true
		return nil
	})
	return input, err
}

func (s *AppService) DeleteGatewayRoute(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateState(func(state *State) error {
		for i, r := range state.GatewayRoutes {
			if r.ID == id {
				state.GatewayRoutes = append(state.GatewayRoutes[:i], state.GatewayRoutes[i+1:]...)
				state.Dirty = true
				return nil
			}
		}
		return errors.New("找不到指定网关规则")
	})
}

func (s *AppService) setAppliedGatewayRoutes(routes []domain.GatewayRoute) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.appliedGatewayRoutes = append([]domain.GatewayRoute(nil), routes...)
}

func (s *AppService) GatewayRoute(path string) (domain.GatewayRoute, bool) {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	for _, r := range s.appliedGatewayRoutes {
		if r.Enabled && r.Path == path {
			return r, true
		}
	}
	return domain.GatewayRoute{}, false
}
