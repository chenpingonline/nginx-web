package acme

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
)

var ErrNotFound = errors.New("ACME 任务不存在")
var ErrConflict = errors.New("任务执行中或配置已被修改，请刷新后再操作")

// Settings is a safe editable view, never the private Record.
type Settings struct {
	Input
	Revision          uint64   `json:"revision"`
	ConfiguredSecrets []string `json:"configured_secrets"`
}

type UpdateInput struct {
	Input
	Revision     uint64   `json:"revision"`
	Retry        bool     `json:"retry"`
	ClearSecrets []string `json:"clear_secrets"`
}

// Convert the original three-provider format without exposing legacy secrets.
func editableInput(r *Record) Input {
	in := clone(r).Input
	if in.DNSConfig == nil {
		in.DNSConfig = map[string]string{}
	}
	if len(in.DNSConfig) == 0 {
		switch in.Provider {
		case "cloudflare":
			in.DNSConfig["CF_DNS_API_TOKEN"] = in.Credentials.Token
		case "alidns":
			in.DNSConfig["ALICLOUD_ACCESS_KEY"], in.DNSConfig["ALICLOUD_SECRET_KEY"] = in.Credentials.AccessID, in.Credentials.Secret
		case "tencentcloud":
			in.DNSConfig["TENCENTCLOUD_SECRET_ID"], in.DNSConfig["TENCENTCLOUD_SECRET_KEY"] = in.Credentials.AccessID, in.Credentials.Secret
		}
	}
	in.Credentials.Token, in.Credentials.AccessID, in.Credentials.Secret = "", "", ""
	return in
}

func secretFields(provider string) map[string]bool {
	fields := map[string]bool{"credentials.eab_kid": true, "credentials.eab_hmac": true}
	if p := providerDefinition(provider); p != nil {
		for _, f := range p.Fields {
			if f.Secret {
				fields["dns_config."+f.Key] = true
			}
		}
	}
	return fields
}

func (m *Manager) Settings(id string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return Settings{}, ErrNotFound
	}
	out := Settings{Input: editableInput(r), Revision: r.Job.Revision, ConfiguredSecrets: []string{}}
	for path := range secretFields(out.Provider) {
		if strings.HasPrefix(path, "dns_config.") {
			key := strings.TrimPrefix(path, "dns_config.")
			if out.DNSConfig[key] != "" {
				out.ConfiguredSecrets = append(out.ConfiguredSecrets, path)
			}
			delete(out.DNSConfig, key)
		}
	}
	if out.Credentials.EABKID != "" {
		out.ConfiguredSecrets = append(out.ConfiguredSecrets, "credentials.eab_kid")
	}
	if out.Credentials.EABHMAC != "" {
		out.ConfiguredSecrets = append(out.ConfiguredSecrets, "credentials.eab_hmac")
	}
	out.Credentials = Credentials{}
	sort.Strings(out.ConfiguredSecrets)
	return out, nil
}

func (m *Manager) Logs(id string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, ErrNotFound
	}
	runs := clone(r).Runs
	if runs == nil {
		runs = []Run{}
	}
	return runs, nil
}

func (m *Manager) Update(id string, update UpdateInput) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.records[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	if m.active == id || update.Revision != stored.Job.Revision {
		return Job{}, ErrConflict
	}
	r := clone(stored)
	previous := editableInput(r)
	in := update.Input
	// Normalize known CA URLs before deciding whether EAB/account can be reused.
	switch in.CA {
	case "letsencrypt":
		in.DirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	case "zerossl":
		in.DirectoryURL = "https://acme.zerossl.com/v2/DV90"
	}
	clear := map[string]bool{}
	allowed := secretFields(in.Provider)
	for _, path := range update.ClearSecrets {
		if !allowed[path] {
			return Job{}, errors.New("不支持清除该凭据字段")
		}
		clear[path] = true
	}
	if in.DNSConfig == nil {
		in.DNSConfig = map[string]string{}
	}
	for key, value := range previous.DNSConfig {
		path := "dns_config." + key
		if in.Provider == previous.Provider && allowed[path] && !clear[path] && in.DNSConfig[key] == "" {
			in.DNSConfig[key] = value
		}
	}
	for path := range clear {
		if strings.HasPrefix(path, "dns_config.") {
			delete(in.DNSConfig, strings.TrimPrefix(path, "dns_config."))
		}
	}
	// Browser edits never write the deprecated credential format.
	in.Credentials.Token, in.Credentials.AccessID, in.Credentials.Secret = "", "", ""
	if in.DirectoryURL == previous.DirectoryURL && in.CA != "letsencrypt" {
		if in.Credentials.EABKID == "" && !clear["credentials.eab_kid"] {
			in.Credentials.EABKID = previous.Credentials.EABKID
		}
		if in.Credentials.EABHMAC == "" && !clear["credentials.eab_hmac"] {
			in.Credentials.EABHMAC = previous.Credentials.EABHMAC
		}
	}
	if clear["credentials.eab_kid"] || in.CA == "letsencrypt" {
		in.Credentials.EABKID = ""
	}
	if clear["credentials.eab_hmac"] || in.CA == "letsencrypt" {
		in.Credentials.EABHMAC = ""
	}
	if err := validate(&in); err != nil {
		return Job{}, err
	}
	accountChanged := in.DirectoryURL != previous.DirectoryURL || in.Email != previous.Email || in.Credentials.EABKID != previous.Credentials.EABKID || in.Credentials.EABHMAC != previous.Credentials.EABHMAC
	newOrder := accountChanged || !reflect.DeepEqual(in.Domains, previous.Domains) || in.KeyType != previous.KeyType
	if accountChanged {
		r.AccountKey, r.Registration = nil, nil
	}
	if newOrder {
		// Deployed files are owned by the certificate store and remain untouched.
		r.Resource, r.RenewAt, r.Pending, r.Force = nil, nil, false, true
	}
	r.Input = in
	r.Job.Name, r.Job.CA, r.Job.Provider, r.Job.KeyType = in.Name, in.CA, in.Provider, in.KeyType
	r.Job.Domains = append([]string(nil), in.Domains...)
	r.Job.Revision++
	r.Job.UpdatedAt = m.now()
	if update.Retry {
		// Keep the existing retry cooldown; editing must not bypass CA rate limits.
		if retryCoolingDown(stored, m.now()) {
			return Job{}, errors.New("请至少间隔一分钟再重试；可以先保存配置")
		}
		r.Job.Enabled, r.Job.Status, r.Job.Failures = true, "queued", 0
		r.Job.NextAttempt, r.Job.Message, r.Force = m.now(), "配置已保存，等待执行；原证书继续使用", true
	}
	if err := m.save(r); err != nil {
		return Job{}, err
	}
	if update.Retry {
		m.signal()
	}
	return r.Job, nil
}

func retryCoolingDown(r *Record, now time.Time) bool {
	if r.LastAttemptAt != nil {
		return now.Before(r.LastAttemptAt.Add(time.Minute))
	}
	if len(r.Runs) > 0 {
		return now.Before(r.Runs[len(r.Runs)-1].StartedAt.Add(time.Minute))
	}
	return false
}
