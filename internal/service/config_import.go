package service

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/pathsecurity"
)

// New servers in one file are previewed and committed as a batch.
// Stable IDs let the UI preview and commit exactly the same generated text.
type ImportSource struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
}

func (src ImportSource) removeBlock(state *State) error {
	for i, file := range state.CustomConfigs {
		if file.Name != src.Name {
			continue
		}
		if file.Content != src.Expected {
			return errors.New("源文件已更改，请刷新后重新编辑")
		}
		if !strings.HasSuffix(file.Name, ".conf") || src.Start < 0 || src.End <= src.Start || src.End > len(file.Content) {
			return errors.New("配置块范围无效")
		}
		rest := file.Content[:src.Start] + file.Content[src.End:]
		if strings.TrimSpace(rest) == "" {
			state.CustomConfigs = append(state.CustomConfigs[:i], state.CustomConfigs[i+1:]...)
		} else {
			state.CustomConfigs[i].Content = rest
		}
		return nil
	}
	return errors.New("找不到自定义配置文件")
}

func (s *AppService) DeleteSourceBlock(source ImportSource) error {
	return s.updateState(func(state *State) error {
		if !state.Settings.MenuEnabled("rules") || !state.Settings.MenuEnabled("config") {
			return errors.New("请先启用代理规则和配置管理")
		}
		return source.removeBlock(state)
	})
}

type ConfigImport struct {
	NewFileName string                `json:"new_file_name,omitempty"`
	Streams     []domain.StreamRule   `json:"streams,omitempty"`
	Source      *ImportSource         `json:"source,omitempty"`
	Pools       []domain.UpstreamPool `json:"pools,omitempty"`
	Rules       []domain.ProxyRule    `json:"rules"`
	Target      string                `json:"target"`
	Location    *domain.LocationRule  `json:"location"`
	Expected    []string              `json:"expected,omitempty"`
}

func (s *AppService) ImportConfig(input ConfigImport, save bool) ([]string, error) {
	return s.importConfig(input, save, nil)
}

func (s *AppService) InspectSourceRule(input ConfigImport) (map[string]any, error) {
	if input.Source == nil || len(input.Rules) != 1 {
		return nil, errors.New("请选择一条自定义 HTTP 规则")
	}
	var rule domain.ProxyRule
	output, err := s.importConfig(input, false, &rule)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rule": rule, "content": output[0]}, nil
}

func (s *AppService) importConfig(input ConfigImport, save bool, normalized *domain.ProxyRule) ([]string, error) {
	var output []string
	prepare := func(state *State) error {
		stream := len(input.Streams) != 0
		if input.NewFileName != "" {
			if err := domain.ValidateCustomConfigName(input.NewFileName); err != nil {
				return err
			}
			if !state.Settings.MenuEnabled("config") || input.Source != nil || input.Location != nil || input.Target != "" ||
				stream != strings.HasSuffix(input.NewFileName, ".stream") {
				return errors.New("自定义文件类型与代理规则不匹配")
			}
			for _, existing := range state.CustomConfigs {
				if strings.EqualFold(existing.Name, input.NewFileName) {
					return errors.New("存在重复的自定义配置文件名")
				}
			}
			if !stream && !state.Settings.MenuEnabled("rules") {
				return errors.New("HTTP 代理菜单已关闭")
			}
		}
		if stream && (input.NewFileName == "" || len(input.Rules) != 0 || len(input.Streams) > 100 || !state.Settings.MenuEnabled("streams")) {
			return errors.New("每次最多转换 100 个 server 块，并需启用 TCP/UDP 代理")
		}
		if input.Source != nil {
			if len(input.Rules) != 1 || input.Location != nil || !state.Settings.MenuEnabled("rules") || !state.Settings.MenuEnabled("config") {
				return errors.New("无法转换此自定义配置")
			}
			if err := input.Source.removeBlock(state); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if len(input.Pools) > 0 {
			if input.Source != nil || input.Location != nil || len(input.Rules)+len(input.Streams) == 0 || len(input.Pools) > 100 {
				return errors.New("转发服务组须与站点一起导入，每次最多 100 个服务组")
			}
			if !state.Settings.MenuEnabled("upstreams") || (!stream && !state.Settings.MenuEnabled("rules")) {
				return errors.New("请先启用代理和转发服务组")
			}
			for _, pool := range input.Pools {
				if !domain.ValidID(pool.ID) || (stream && pool.Protocol != "stream") || (!stream && pool.Protocol != "http") {
					return errors.New("转发服务组标识或协议无效")
				}
				pool.CreatedAt, pool.UpdatedAt = now, now
				state.UpstreamPools = append(state.UpstreamPools, pool)
			}
		}
		var ids []string
		if stream {
			seen := make(map[string]bool)
			for _, r := range state.StreamRules {
				seen[r.ID] = true
			}
			for _, r := range input.Streams {
				if !domain.ValidID(r.ID) || seen[r.ID] {
					return errors.New("TCP/UDP 规则标识无效或已存在")
				}
				seen[r.ID] = true
				r.CreatedAt, r.UpdatedAt = now, now
				state.StreamRules = append(state.StreamRules, r)
				ids = append(ids, r.ID)
			}
		} else if input.Location != nil {
			if !domain.ValidID(input.Target) || len(input.Rules) != 0 {
				return errors.New("请选择一个目标代理")
			}
			loc := *input.Location
			if !domain.ValidID(loc.ID) || (loc.Match == "" || loc.Match == "prefix") && loc.Path == "/" {
				return errors.New("根路径已存在或路径标识无效")
			}
			for i := range state.Rules {
				r := &state.Rules[i]
				if r.ID != input.Target {
					continue
				}
				if r.RedirectToHTTPS {
					return errors.New("该代理已启用整站跳转，无法添加生效的路径")
				}
				for _, other := range r.Locations {
					if other.ID == loc.ID || other.Path == loc.Path && other.Match == loc.Match {
						return errors.New("该代理已有相同路径或标识")
					}
				}
				r.Locations = append(r.Locations, loc)
				r.UpdatedAt = now
				ids = append(ids, r.ID)
				break
			}
			if len(ids) == 0 {
				return errors.New("目标代理不存在")
			}
		} else {
			if len(input.Rules) == 0 || len(input.Rules) > 100 || input.Target != "" {
				return errors.New("每次可导入 1 到 100 个 server 块")
			}
			seen := map[string]bool{}
			for _, r := range state.Rules {
				seen[r.ID] = true
			}
			for _, r := range input.Rules {
				if !domain.ValidID(r.ID) || seen[r.ID] {
					return errors.New("规则标识无效或已导入")
				}
				seen[r.ID] = true
				r.CreatedAt, r.UpdatedAt = now, now
				state.Rules = append(state.Rules, r)
				ids = append(ids, r.ID)
			}
		}
		domain.ApplyStateDefaults(state)
		if err := domain.ValidateRuntimePorts(*state); err != nil {
			return err
		}
		if err := domain.ValidateState(*state); err != nil {
			return err
		}
		if err := pathsecurity.ValidateState(*state); err != nil {
			return err
		}
		if normalized != nil {
			*normalized = state.Rules[len(state.Rules)-1]
		}
		for _, pool := range input.Pools {
			content, err := s.nginx.FormConfig(*state, "upstreams", pool.ID)
			if err != nil {
				return err
			}
			output = append(output, content)
		}
		for _, id := range ids {
			content, err := s.nginx.RuleConfig(*state, id, stream)
			if err != nil {
				return err
			}
			output = append(output, content)
		}
		if save && !slices.Equal(input.Expected, output) {
			return errors.New("规则配置已变化，请重新预览")
		}
		return nil
	}
	if save {
		err := s.updateState(prepare)
		return output, err
	}
	state := s.store.Snapshot()
	err := prepare(&state)
	return output, err
}
