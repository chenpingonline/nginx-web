package service

import (
	"errors"
	"fmt"
	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/platform"
	"slices"
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
	return input, s.changeGatewayRoute(input.ID, &input, id != "")
}

func (s *AppService) DeleteGatewayRoute(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changeGatewayRoute(id, nil, true)
}

// Update the selected route in both lists in one durable write. Other restored
// or legacy draft routes retain their previous activation state.
func (s *AppService) changeGatewayRoute(id string, route *domain.GatewayRoute, requireExisting bool) error {
	edit := func(routes []domain.GatewayRoute) []domain.GatewayRoute {
		for i := range routes {
			if routes[i].ID == id {
				if route == nil {
					return slices.Delete(routes, i, i+1)
				}
				routes[i] = *route
				return routes
			}
		}
		if route != nil {
			routes = append(routes, *route)
		}
		return routes
	}
	s.authMu.RLock()
	live := append([]domain.GatewayRoute{}, s.appliedGatewayRoutes...)
	s.authMu.RUnlock()
	live = edit(live)
	err := s.updateState(func(state *State) error {
		if requireExisting && !slices.ContainsFunc(state.GatewayRoutes, func(r domain.GatewayRoute) bool { return r.ID == id }) {
			return errors.New("找不到指定网关规则")
		}
		state.GatewayRoutes = edit(state.GatewayRoutes)
		state.LiveGatewayRoutes = &live
		return nil
	})
	if err == nil {
		s.setAppliedGatewayRoutes(live)
	}
	return err
}

func gatewayBaseline(base, current State) State {
	if current.LiveGatewayRoutes != nil {
		base.GatewayRoutes = *current.LiveGatewayRoutes
	}
	base.LiveGatewayRoutes = current.LiveGatewayRoutes
	return base
}

func (s *AppService) setAppliedGatewayRoutes(routes []domain.GatewayRoute) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.appliedGatewayRoutes = append([]domain.GatewayRoute(nil), routes...)
}

func (s *AppService) GatewayRoute(path string) (domain.GatewayRoute, bool) {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	if slices.Contains(s.appliedMenus, "gateway") {
		return domain.GatewayRoute{}, false
	}
	for _, r := range s.appliedGatewayRoutes {
		if r.Enabled && r.Path == path {
			return r, true
		}
	}
	return domain.GatewayRoute{}, false
}
