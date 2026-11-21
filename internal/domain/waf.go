package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/crscatalog"
)

const DefaultWAFBodyMiB = 8
const MaxWAFBodyMiB = 128

type WAFInstance struct {
	BodyLimitMiB    int              `json:"body_limit_mib,omitempty"`
	BodyLimitAction string           `json:"body_limit_action,omitempty"`
	UploadSites     []string         `json:"upload_sites,omitempty"`
	Exclusions      map[string][]int `json:"exclusions,omitempty"`
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Mode            string           `json:"mode"`
	Sites           []string         `json:"sites"`
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
		if p.BodyLimitMiB < 0 || p.BodyLimitMiB > MaxWAFBodyMiB || (p.BodyLimitAction != "" && p.BodyLimitAction != "reject" && p.BodyLimitAction != "partial") {
			return errors.New("请求体检查上限或超限处理无效")
		}
		for _, site := range p.UploadSites {
			if !slices.Contains(p.Sites, site) {
				return errors.New("上传兼容仅适用于已绑定站点")
			}
		}
		for site, rules := range p.Exclusions {
			if !slices.Contains(p.Sites, site) {
				return errors.New("规则排除项必须属于已绑定站点")
			}
			seen := map[int]bool{}
			for _, id := range rules {
				if !crscatalog.Known(id) || seen[id] {
					return fmt.Errorf("无效或重复的 CRS 规则 ID %d", id)
				}
				seen[id] = true
			}
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
