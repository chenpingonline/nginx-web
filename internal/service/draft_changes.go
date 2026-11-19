package service

import (
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

// Dirty is a cache of content comparison, never evidence of a known baseline.
func (s *AppService) reconcileDraft(state *State) {
	if applied, known := s.appliedState(*state); known {
		state.Dirty = !sameToggleConfiguration(*state, applied)
		if !state.Dirty {
			state.DraftRevisionID = ""
		}
	} else {
		state.Dirty = true
	}
}

func (s *AppService) updateState(fn func(*State) error) error {
	return s.store.Update(func(state *State) error {
		if err := fn(state); err != nil {
			return err
		}
		domain.ApplyStateDefaults(state)
		s.reconcileDraft(state)
		return nil
	})
}

func (s *AppService) replaceState(state State) error {
	domain.ApplyStateDefaults(&state)
	s.reconcileDraft(&state)
	return s.store.Replace(state)
}

type DraftFieldChange struct {
	Path      string `json:"path"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Sensitive bool   `json:"sensitive"`
}

type DraftChange struct {
	Section   string             `json:"section"`
	Name      string             `json:"name"`
	Operation string             `json:"operation"`
	Fields    []DraftFieldChange `json:"fields"`
}

type DraftChanges struct {
	Status  string        `json:"status"`
	Changes []DraftChange `json:"changes"`
}

func (s *AppService) DraftChanges() DraftChanges {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := s.store.Snapshot()
	applied, known := s.appliedState(draft)
	result := DraftChanges{Status: "unknown", Changes: []DraftChange{}}
	if !known {
		if draft.LastAppliedAt == nil {
			result.Status = "uninitialized"
		}
		return result
	}
	result.Status = "synced"
	before, after := configurationMap(applied), configurationMap(draft)
	sections := []string{"rules", "stream_rules", "gateway_routes", "rule_groups", "upstream_pools", "rate_limit_policies", "auth_profiles", "custom_configs", "settings"}
	for _, section := range sections {
		left, right := before[section], after[section]
		if reflect.DeepEqual(left, right) {
			continue
		}
		if section == "settings" {
			result.Changes = append(result.Changes, DraftChange{Section: section, Name: "全局设置", Operation: "modified", Fields: changedFields(section, "", left, right)})
			continue
		}
		oldItems, oldOrder := configurationItems(left)
		newItems, newOrder := configurationItems(right)
		keys := append([]string{}, newOrder...)
		for _, key := range oldOrder {
			if _, ok := newItems[key]; !ok {
				keys = append(keys, key)
			}
		}
		for _, key := range keys {
			old, existed := oldItems[key]
			next, exists := newItems[key]
			if reflect.DeepEqual(old, next) {
				continue
			}
			operation := "modified"
			item := next
			if !existed {
				operation = "added"
			}
			if !exists {
				operation = "deleted"
				item = old
			}
			name, _ := item["name"].(string)
			if name == "" {
				name = key
			}
			fields := changedFields(section, "", old, next)
			if operation != "modified" {
				fields = entitySummary(fields)
			}
			result.Changes = append(result.Changes, DraftChange{Section: section, Name: name, Operation: operation, Fields: fields})
		}
		// Order can affect routing priority; do not silently sort it away.
		if len(oldOrder) == len(newOrder) && !reflect.DeepEqual(oldOrder, newOrder) {
			sameSet := true
			for _, key := range oldOrder {
				if _, ok := newItems[key]; !ok {
					sameSet = false
				}
			}
			if sameSet {
				result.Changes = append(result.Changes, DraftChange{Section: section, Name: "排列顺序", Operation: "modified", Fields: []DraftFieldChange{{Path: "order", Before: "原有顺序", After: "顺序已调整"}}})
			}
		}
	}
	if len(result.Changes) > 0 {
		result.Status = "pending"
	}
	return result
}

func configurationMap(state State) map[string]any {
	raw, _ := json.Marshal(configurationContent(state))
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	return value
}

func configurationItems(value any) (map[string]map[string]any, []string) {
	items := map[string]map[string]any{}
	order := []string{}
	list, _ := value.([]any)
	for i, raw := range list {
		item, _ := raw.(map[string]any)
		key, _ := item["id"].(string)
		if key == "" {
			key, _ = item["name"].(string)
		}
		if key == "" {
			key = strconv.Itoa(i + 1)
		}
		items[key] = item
		order = append(order, key)
	}
	return items, order
}

func changedFields(section, path string, before, after any) []DraftFieldChange {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	left, leftOK := before.(map[string]any)
	right, rightOK := after.(map[string]any)
	if leftOK || rightOK {
		keys := map[string]bool{}
		for key := range left {
			keys[key] = true
		}
		for key := range right {
			keys[key] = true
		}
		sorted := []string{}
		for key := range keys {
			sorted = append(sorted, key)
		}
		sort.Strings(sorted)
		fields := []DraftFieldChange{}
		for _, key := range sorted {
			if key == "id" || key == "created_at" || key == "updated_at" {
				continue
			}
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			fields = append(fields, changedFields(section, nextPath, left[key], right[key])...)
		}
		return fields
	}
	leftList, isLeftList := before.([]any)
	rightList, isRightList := after.([]any)
	if (isLeftList || isRightList) && section != "auth_profiles" {
		fields := []DraftFieldChange{}
		count := len(leftList)
		if len(rightList) > count {
			count = len(rightList)
		}
		for i := 0; i < count; i++ {
			var old, next any
			if i < len(leftList) {
				old = leftList[i]
			}
			if i < len(rightList) {
				next = rightList[i]
			}
			fields = append(fields, changedFields(section, path+"["+strconv.Itoa(i+1)+"]", old, next)...)
		}
		return fields
	}
	sensitive := section == "auth_profiles" || strings.Contains(path, "password") || strings.Contains(path, "secret") || strings.Contains(path, "token") || strings.Contains(path, "header") || (section == "custom_configs" && path == "content")
	oldValue, newValue := changeValue(path, before, sensitive), changeValue(path, after, sensitive)
	sensitive = sensitive || strings.Contains(oldValue, "内容隐藏") || strings.Contains(newValue, "内容隐藏")
	return []DraftFieldChange{{Path: path, Before: oldValue, After: newValue, Sensitive: sensitive}}
}

// Never return hashes, arbitrary configuration text or credential-bearing URLs.
func changeValue(path string, value any, sensitive bool) string {
	if value == nil {
		return "未设置"
	}
	if sensitive {
		return "已设置（内容隐藏）"
	}
	switch v := value.(type) {
	case bool:
		if v {
			return "开启"
		}
		return "关闭"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		return "列表（" + strconv.Itoa(len(v)) + " 项）"
	case string:
		if v == "" {
			return "未设置"
		}
		if strings.Contains(v, "://") {
			u, err := url.Parse(v)
			if err != nil {
				return "已设置（内容隐藏）"
			}
			if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return "已设置（内容隐藏）"
			}
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			return u.String()
		}
		safe := map[string]bool{"name": true, "compatibility": true, "tls_fingerprint": true, "tls_trust_target": true, "domains": true, "path": true, "target": true, "upstream_host": true, "upstream_scheme": true, "listen_address": true, "protocol": true, "strategy": true, "backend_type": true, "static_path": true, "certificate_id": true, "rate_limit_policy_id": true, "upstream_pool_id": true, "tls_mode": true}
		parts := strings.Split(path, ".")
		key := parts[len(parts)-1]
		if bracket := strings.IndexByte(key, '['); bracket >= 0 {
			key = key[:bracket]
		}
		if safe[key] {
			return v
		}
		return "已设置（内容隐藏）"
	}
	return "已设置（内容隐藏）"
}

// New/deleted entities need identification and routing facts, not every default.
func entitySummary(fields []DraftFieldChange) []DraftFieldChange {
	keys := map[string]bool{"enabled": true, "domains": true, "listen_address": true, "listen_port": true, "protocol": true, "tls": true, "tls_mode": true, "target": true, "path": true, "upstream_host": true, "upstream_port": true, "upstream_scheme": true, "upstream_pool_id": true, "certificate_id": true, "strategy": true, "servers": true, "users": true, "settings": true, "content": true}
	result := []DraftFieldChange{}
	for _, field := range fields {
		root := strings.Split(field.Path, ".")[0]
		if bracket := strings.IndexByte(root, '['); bracket >= 0 {
			root = root[:bracket]
		}
		if keys[root] || field.Path == "root_location.backend_type" || field.Path == "root_location.static_path" {
			result = append(result, field)
		}
	}
	return result
}
