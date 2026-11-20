package domain

import (
	"errors"
	"fmt"
	"strings"
)

type WAFInstance struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Mode  string   `json:"mode"`
	Sites []string `json:"sites"`
}

func validateWAF(state State) error {
	sites := map[string]bool{}
	for _, r := range state.Rules {
		sites[r.ID] = true
	}
	ids := map[string]bool{}
	bound := map[string]bool{}
	for _, p := range state.WAFInstances {
		if !ValidID(p.ID) || ids[p.ID] || strings.TrimSpace(p.Name) == "" || len([]rune(p.Name)) > 80 {
			return errors.New("WAF 实例名称或编号无效")
		}
		ids[p.ID] = true
		if p.Mode != "detect" && p.Mode != "block" {
			return errors.New("未知的 WAF 模式")
		}
		for _, id := range p.Sites {
			if !sites[id] || bound[id] {
				return fmt.Errorf("站点 %s 不存在或已绑定 WAF", id)
			}
			bound[id] = true
		}
	}
	return nil
}
