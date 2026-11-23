package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/pathsecurity"
)

// The comment carries non-rendered fields such as disabled routes and names.
// The visible configuration must agree with the same renderer before opening a form.
const formPrefix = "# nginx-web-form-v1 "

type ConfigForm struct {
	Paths        map[string]string `json:"paths,omitempty"`
	Dependencies string            `json:"dependencies,omitempty"`
	Kind         string            `json:"kind"`
	ID           string            `json:"id"`
	Value        json.RawMessage   `json:"value"`
}

func formObject[T any](items []T, id string, key func(T) string, replacement json.RawMessage) ([]T, any, error) {
	for index, v := range items {
		if key(v) != id {
			continue
		}
		if replacement != nil {
			var next T
			if err := json.Unmarshal(replacement, &next); err != nil {
				return items, nil, err
			}
			if key(next) != id {
				return items, nil, errors.New("表单标识不一致")
			}
			items[index] = next
		}
		return items, items[index], nil
	}
	if replacement != nil {
		var next T
		if err := json.Unmarshal(replacement, &next); err != nil {
			return items, nil, err
		}
		if key(next) == id {
			return append(items, next), next, nil
		}
	}
	return items, nil, errors.New("找不到对应的表单")
}
func selectForm(st *State, f ConfigForm) (any, error) {
	if f.Kind != "settings" && !domain.ValidID(f.ID) {
		return nil, errors.New("配置标识无效")
	}
	var value any
	var err error
	switch f.Kind {
	case "settings":
		if f.ID != "" {
			return nil, errors.New("全局设置标识无效")
		}
		if f.Value != nil {
			var v Settings
			if err = json.Unmarshal(f.Value, &v); err != nil {
				return nil, err
			}
			st.Settings = v
		}
		return st.Settings, nil
	case "rule-groups":
		st.RuleGroups, value, err = formObject(st.RuleGroups, f.ID, func(v domain.RuleGroup) string { return v.ID }, f.Value)
	case "rules":
		st.Rules, value, err = formObject(st.Rules, f.ID, func(v ProxyRule) string { return v.ID }, f.Value)
	case "streams":
		st.StreamRules, value, err = formObject(st.StreamRules, f.ID, func(v StreamRule) string { return v.ID }, f.Value)
	case "upstreams":
		st.UpstreamPools, value, err = formObject(st.UpstreamPools, f.ID, func(v UpstreamPool) string { return v.ID }, f.Value)
	case "rate-limits":
		st.RateLimitPolicies, value, err = formObject(st.RateLimitPolicies, f.ID, func(v RateLimitPolicy) string { return v.ID }, f.Value)
	case "waf":
		st.WAFInstances, value, err = formObject(st.WAFInstances, f.ID, func(v domain.WAFInstance) string { return v.ID }, f.Value)
	case "auth-profiles":
		// Work only on public values. A pasted password or password hash is never imported.
		profiles := domain.PublicState(State{AuthProfiles: st.AuthProfiles}).AuthProfiles
		profiles, value, err = formObject(profiles, f.ID, func(p domain.AuthProfile) string { return p.ID }, f.Value)
		if err != nil {
			return nil, err
		}
		profile := value.(domain.AuthProfile)
		for _, user := range profile.Users {
			if user.PasswordHash != "" {
				return nil, errors.New("配置中不能包含认证密码哈希")
			}
		}
		if err = domain.ValidatePublicAuthProfile(profile); err != nil {
			return nil, err
		}
		if f.Value != nil {
			st.AuthProfiles = profiles
		}

	default:
		return nil, errors.New("不支持此配置表单")
	}
	return value, err
}
func (s *AppService) formMenuEnabled(kind string) bool {
	if kind == "rule-groups" {
		kind = "rules"
	}
	return kind == "settings" || s.MenuEnabled(kind)
}

func (s *AppService) ExportForm(kind, id string) (string, error) {
	if !s.formMenuEnabled(kind) {
		return "", errors.New("此表单菜单已关闭")
	}
	st := s.store.Snapshot()
	f := ConfigForm{Kind: kind, ID: id}
	value, err := selectForm(&st, f)
	if err != nil {
		return "", err
	}
	f.Dependencies, err = formDependencies(st, value)
	if err != nil {
		return "", err
	}
	f.Value, err = json.Marshal(value)
	if err != nil {
		return "", err
	}
	content, err := s.nginx.FormConfig(st, kind, id)
	if err != nil {
		return "", err
	}
	f.Paths = formPaths(s.paths, st, content)
	return EncodeForm(f, content)
}

func EncodeForm(f ConfigForm, content string) (string, error) {
	header, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	data := base64.RawStdEncoding.EncodeToString(header)
	var out strings.Builder
	for len(data) > 0 {
		n := min(1800, len(data))
		out.WriteString(formPrefix + data[:n] + "\n")
		data = data[n:]
	}
	out.WriteString(content)
	if out.Len() > 1048576 {
		return "", errors.New("配置超过 1 MB")
	}
	return out.String(), nil
}
func (s *AppService) ReadForm(text string) (ConfigForm, error) {
	var f ConfigForm
	if len(text) > 1048576 {
		return f, errors.New("配置超过 1 MB")
	}
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	var header strings.Builder
	for strings.HasPrefix(text, formPrefix) {
		line, rest, _ := strings.Cut(text, "\n")
		header.WriteString(strings.TrimPrefix(line, formPrefix))
		text = rest
	}
	data, err := base64.RawStdEncoding.DecodeString(header.String())
	if err != nil || json.Unmarshal(data, &f) != nil || len(f.Value) == 0 {
		return ConfigForm{}, errors.New("表单配置注释不完整")
	}
	result, content, err := s.PreviewForm(f)
	if err != nil {
		return ConfigForm{}, err
	}
	if result.Dependencies != f.Dependencies {
		return ConfigForm{}, errors.New("引用的证书、认证、WAF、限流、分组或上游定义已变化，请核对依赖")
	}
	original, err := originalFormText(content, result.Paths, f.Paths)
	if err != nil {
		return ConfigForm{}, err
	}
	if strings.TrimSpace(original) != strings.TrimSpace(text) {
		return ConfigForm{}, errors.New("配置正文、关联资源或运行路径与表单不一致，请核对原文")
	}
	return result, nil
}

// PreviewForm normalizes a candidate parsed from plain Nginx directives without writing it.
func (s *AppService) PreviewForm(f ConfigForm) (ConfigForm, string, error) {
	if !s.formMenuEnabled(f.Kind) {
		return ConfigForm{}, "", errors.New("此表单菜单已关闭")
	}
	st := s.store.Snapshot()
	if _, err := selectForm(&st, f); err != nil {
		return ConfigForm{}, "", err
	}
	// Credentials are supplied only when the authentication editor is saved.
	if f.Kind == "auth-profiles" {
		content, err := s.nginx.FormConfig(st, f.Kind, f.ID)
		if err != nil {
			return ConfigForm{}, "", err
		}
		value, err := selectForm(&st, ConfigForm{Kind: f.Kind, ID: f.ID})
		if err != nil {
			return ConfigForm{}, "", err
		}
		f.Paths = formPaths(s.paths, st, content)
		f.Value, err = json.Marshal(value)
		return f, content, err
	}
	domain.ApplyStateDefaults(&st)
	if err := domain.ValidateState(st); err != nil {
		return ConfigForm{}, "", err
	}
	if err := domain.ValidateRuntimePorts(st); err != nil {
		return ConfigForm{}, "", err
	}
	if err := pathsecurity.ValidateState(st); err != nil {
		return ConfigForm{}, "", err
	}
	content, err := s.nginx.FormConfig(st, f.Kind, f.ID)
	if err != nil {
		return ConfigForm{}, "", err
	}
	value, err := selectForm(&st, ConfigForm{Kind: f.Kind, ID: f.ID})
	if err != nil {
		return ConfigForm{}, "", err
	}
	f.Dependencies, err = formDependencies(st, value)
	if err != nil {
		return ConfigForm{}, "", err
	}
	f.Paths = formPaths(s.paths, st, content)
	f.Value, err = json.Marshal(value)
	return f, content, err
}

// CreateImportedForm retains IDs used by related forms. A concurrent create
// must fail rather than turn a reviewed addition into an update.
func (s *AppService) CreateImportedForm(f ConfigForm) (any, error) {
	if !s.formMenuEnabled(f.Kind) {
		return nil, errors.New("此表单菜单已关闭")
	}
	if f.Kind == "auth-profiles" || f.Kind == "settings" || len(f.Value) == 0 {
		return nil, errors.New("此表单不支持新建")
	}
	var value any
	err := s.updateState(func(st *State) error {
		if _, err := selectForm(st, ConfigForm{Kind: f.Kind, ID: f.ID}); err == nil {
			return errors.New("同一标识的配置已存在，请重新打开表单")
		}
		var err error
		// Editor submissions carry user-editable fields; stamp newly created resources
		// exactly as their regular creation endpoints do.
		switch f.Kind {
		case "rules", "streams", "upstreams", "rate-limits":
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(f.Value, &fields); err != nil || fields == nil {
				return errors.New("配置表单无效")
			}
			now, _ := json.Marshal(time.Now().UTC())
			fields["created_at"], fields["updated_at"] = now, now
			f.Value, err = json.Marshal(fields)
			if err != nil {
				return err
			}
		}
		value, err = selectForm(st, f)
		if err != nil {
			return err
		}
		domain.ApplyStateDefaults(st)
		return pathsecurity.ValidateState(*st)
	})
	return value, err
}
