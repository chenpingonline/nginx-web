package service

import (
	"errors"

	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/pathsecurity"
)

// An import contains either new HTTP servers or a single location to append.
// Stable IDs let the UI preview and commit exactly the same generated text.
type ConfigImport struct {
	Rules    []domain.ProxyRule   `json:"rules"`
	Target   string               `json:"target"`
	Location *domain.LocationRule `json:"location"`
	Expected []string             `json:"expected,omitempty"`
}

func (s *AppService) ImportConfig(input ConfigImport, save bool) ([]string, error) {
	var output []string
	prepare := func(state *State) error {
		now := time.Now().UTC()
		var ids []string
		if input.Location != nil {
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
		for i, id := range ids {
			content, err := s.nginx.RuleConfig(*state, id, false)
			if err != nil {
				return err
			}
			if save && (len(input.Expected) != len(ids) || input.Expected[i] != content) {
				return errors.New("规则配置已变化，请重新预览")
			}
			output = append(output, content)
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
