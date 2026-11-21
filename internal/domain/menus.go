package domain

import (
	"fmt"
	"slices"
)

func (s Settings) MenuEnabled(id string) bool {
	return !slices.Contains(s.HiddenMenus, id)
}

// Keep saved rules intact. Apply the switches only to the effective configuration.
func MenuRuntimeState(state State) State {
	if !state.Settings.MenuEnabled("rules") {
		state.Rules = nil
	}
	if !state.Settings.MenuEnabled("streams") {
		state.StreamRules = nil
	}
	if !state.Settings.MenuEnabled("upstreams") {
		state.UpstreamPools = nil
	}
	if !state.Settings.MenuEnabled("rate-limits") {
		state.RateLimitPolicies = nil
	}
	if !state.Settings.MenuEnabled("waf") {
		state.WAFInstances = nil
	}
	if !state.Settings.MenuEnabled("config") {
		state.CustomConfigs = nil
	}
	return state
}

func validateMenus(state State) error {
	for _, id := range state.Settings.HiddenMenus {
		switch id {
		case "dashboard", "rules", "gateway", "streams", "upstreams", "auth-profiles", "rate-limits", "certificates", "errors", "topology", "waf", "logs", "backup", "revisions", "config":
		default:
			return fmt.Errorf("无法隐藏菜单 %q", id)
		}
	}
	if state.Settings.MenuEnabled("upstreams") {
		return nil
	}
	active := MenuRuntimeState(state)
	for _, r := range active.Rules {
		if !r.Enabled {
			continue
		}
		used := r.UpstreamPoolID != "" || r.RootLocation.UpstreamPoolID != ""
		for _, l := range r.Locations {
			used = used || (l.Enabled && l.Settings.UpstreamPoolID != "")
		}
		if used {
			return fmt.Errorf("HTTP 规则「%s」正在使用转发服务组，请先调整该规则或关闭 HTTP(S) 代理菜单", r.Name)
		}
	}
	for _, r := range active.StreamRules {
		if !r.Enabled {
			continue
		}
		used := r.UpstreamPoolID != ""
		for _, route := range r.SNIRoutes {
			used = used || route.UpstreamPoolID != ""
		}
		if used {
			return fmt.Errorf("Stream 规则「%s」正在使用转发服务组，请先调整该规则或关闭 TCP/UDP 代理菜单", r.Name)
		}
	}
	return nil
}
