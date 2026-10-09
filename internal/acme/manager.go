package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/fileutil"
	"github.com/go-acme/lego/v5/certificate"
)

type DeployFunc func(id, name string, resource *certificate.Resource) (string, error)
type IssueFunc func(context.Context, *Record, func() error, func(string)) error
type Manager struct {
	mu      sync.Mutex
	dir     string
	records map[string]*Record
	active  string
	deploy  DeployFunc
	issue   IssueFunc
	now     func() time.Time
	wake    chan struct{}
}

func New(dir string, deploy DeployFunc) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	m := &Manager{dir: dir, records: map[string]*Record{}, deploy: deploy, issue: issue, now: time.Now, wake: make(chan struct{}, 1)}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		var r Record
		if json.Unmarshal(b, &r) != nil || !domain.ValidID(r.Job.ID) || filepath.Base(p) != r.Job.ID+".json" {
			return nil, errors.New("ACME 任务文件损坏")
		}
		// Older releases exposed the Let's Encrypt staging endpoint as a named CA.
		// Preserve those tasks as ordinary custom-directory jobs while removing
		// staging as a selectable and API-supported certificate authority.
		migrated := r.Input.CA == "staging"
		if migrated {
			r.Input.CA = "custom"
			r.Job.CA = "custom"
		}
		if err := validate(&r.Input); err != nil {
			return nil, fmt.Errorf("ACME 任务 %s 配置无效", r.Job.ID)
		}
		recovered := r.Job.Status == "running"
		if r.LastAttemptAt == nil && len(r.Runs) == 0 && (recovered || r.Job.Status == "failed") {
			at := r.Job.UpdatedAt
			r.LastAttemptAt = &at
			migrated = true
		}
		if r.Job.Revision == 0 {
			r.Job.Revision = 1
			migrated = true
		}
		if recovered {
			if len(r.Runs) > 0 {
				run := &r.Runs[len(r.Runs)-1]
				if run.Status == "running" {
					at := m.now().UTC()
					run.FinishedAt = &at
					run.Status = "interrupted"
					appendEntry(&r, at, "warning", "上次执行中断，等待恢复", diagnostic("interrupted"))
				}
			}

			r.Job.Status = "queued"
			r.Job.Message = "上次任务中断，等待恢复"
			r.Job.NextAttempt = m.now()
		}
		if migrated || recovered {
			if err := m.save(&r); err != nil {
				return nil, err
			}
		} else {
			r.Job.DeploymentPending = r.Pending
			if r.LastAttemptAt != nil {
				at := r.LastAttemptAt.Add(time.Minute)
				if r.Job.FailureCode == "ca_rate_limit" && r.Job.NextAttempt.After(at) {
					at = r.Job.NextAttempt
				}
				r.Job.RetryAfter = &at
			}
			m.records[r.Job.ID] = &r
		}
	}
	return m, nil
}
func clone(r *Record) *Record {
	b, _ := json.Marshal(r)
	var c Record
	_ = json.Unmarshal(b, &c)
	return &c
}
func (m *Manager) save(r *Record) error {
	r.Job.DeploymentPending = r.Pending
	if r.LastAttemptAt != nil {
		at := r.LastAttemptAt.Add(time.Minute)
		if r.Job.FailureCode == "ca_rate_limit" && r.Job.NextAttempt.After(at) {
			at = r.Job.NextAttempt
		}
		r.Job.RetryAfter = &at
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err = fileutil.WriteFileAtomic(filepath.Join(m.dir, r.Job.ID+".json"), b, 0600); err != nil {
		return err
	}
	m.records[r.Job.ID] = clone(r)
	return nil
}
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) Create(in Input) (Job, error) {
	if err := validate(&in); err != nil {
		return Job{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) >= 100 {
		return Job{}, errors.New("最多支持 100 个 ACME 任务")
	}
	r := &Record{Input: in, Job: Job{ID: domain.RandomID(), Name: in.Name, CA: in.CA, Domains: in.Domains, Provider: in.Provider, KeyType: in.KeyType, Revision: 1, Enabled: true, Status: "queued", Message: "等待申请", NextAttempt: m.now(), UpdatedAt: m.now()}}
	if err := m.save(r); err != nil {
		return Job{}, err
	}
	m.signal()
	return r.Job, nil
}
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := []Job{}
	for _, r := range m.records {
		j := r.Job
		j.Domains = append([]string(nil), j.Domains...)
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs
}

// Reissue retains the private account and DNS configuration, but starts a new
// order: Renew would reuse the old certificate's domains. The deployed files
// remain untouched until the replacement has been issued and deployed.
func (m *Manager) Reissue(id string, domains []string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.records[id]
	if !ok {
		return Job{}, errors.New("ACME 任务不存在")
	}
	if m.active == id {
		return Job{}, errors.New("任务执行中，请等待完成后修改")
	}
	if retryCoolingDown(stored, m.now()) {
		return Job{}, errors.New("请等待重试冷却结束后再重新签发")
	}
	r := clone(stored)
	r.Input.Domains = append([]string(nil), domains...)
	if err := validate(&r.Input); err != nil {
		return Job{}, err
	}
	r.Job.Domains = append([]string(nil), r.Input.Domains...)
	r.Job.Revision++
	r.Resource = nil
	r.Pending = false
	r.RenewAt = nil
	r.Force = true
	r.Job.Enabled = true
	r.Job.Status = "queued"
	r.Job.Message = "域名已修改，等待重新签发；原证书继续使用"
	r.Job.Failures = 0
	r.Job.NextAttempt = m.now()
	r.Job.UpdatedAt = m.now()
	if err := m.save(r); err != nil {
		return Job{}, err
	}
	m.signal()
	return r.Job, nil
}
func (m *Manager) Manages(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.Job.CertificateID == id || r.Job.ID == id {
			return true
		}
	}
	return false
}
func (m *Manager) Action(id, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return errors.New("ACME 任务不存在")
	}
	if m.active == id {
		return errors.New("任务执行中，请等待完成后操作")
	}
	r = clone(r)
	switch action {
	case "retry", "retry_deploy":
		if action == "retry_deploy" && !r.Pending {
			return errors.New("没有待部署的证书，请使用重试申请或立即续期")
		}
		if action == "retry_deploy" {
			if cert, err := leaf(r.Resource); err != nil || !cert.NotAfter.After(m.now()) {
				return errors.New("待部署证书已失效，请编辑配置后重新申请")
			}
		}
		if !r.Job.Enabled {
			return errors.New("请先启用自动续期")
		}
		if retryCoolingDown(r, m.now()) {
			if r.Job.FailureCode == "ca_rate_limit" {
				return errors.New("证书机构限制了申请频率，请等待任务显示的冷却时间结束")
			}
			return errors.New("请至少间隔一分钟再重试")
		}
		r.Job.NextAttempt = m.now()
		r.Job.Status = "queued"
		r.Force = true
		if r.Pending {
			r.Job.Message = "等待重试部署，复用已签发证书"
		} else {
			r.Job.Message = "等待执行"
		}
	case "pause":
		r.Job.Enabled = false
		if r.Job.Status != "failed" {
			r.Job.Message = "自动续期已暂停"
		}
	case "resume":
		r.Job.Enabled = true
		if r.Job.FailureCode != "ca_rate_limit" || !r.Job.NextAttempt.After(m.now()) {
			r.Job.NextAttempt = m.now()
		}
		if r.Job.Status != "failed" {
			r.Job.Message = "自动续期已启用"
		}
	case "delete":
		if err := os.Remove(filepath.Join(m.dir, id+".json")); err != nil {
			return err
		}
		delete(m.records, id)
		return nil
	default:
		return errors.New("不支持的任务操作")
	}
	r.Job.UpdatedAt = m.now()
	if err := m.save(r); err != nil {
		return err
	}
	m.signal()
	return nil
}

// RemoveForCertificate stops future renewals before the certificate is removed.
// Check all matching jobs first so an active renewal leaves every job untouched.
func (m *Manager) RemoveForCertificate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for jobID, r := range m.records {
		if r.Job.CertificateID != id && r.Job.ID != id {
			continue
		}
		if m.active == jobID {
			return errors.New("关联 ACME 任务执行中，请等待完成后删除证书")
		}
		ids = append(ids, jobID)
	}
	for _, jobID := range ids {
		if err := os.Remove(filepath.Join(m.dir, jobID+".json")); err != nil {
			return err
		}
		delete(m.records, jobID)
	}
	return nil
}
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		m.step(ctx)
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
	}
}
func (m *Manager) step(ctx context.Context) {
	m.mu.Lock()
	var r *Record
	for _, candidate := range m.records {
		if candidate.Job.Enabled && !candidate.Job.NextAttempt.After(m.now()) && (r == nil || candidate.Job.NextAttempt.Before(r.Job.NextAttempt)) {
			r = clone(candidate)
		}
	}
	if r == nil || m.active != "" {
		m.mu.Unlock()
		return
	}
	m.active = r.Job.ID
	attemptAt := m.now().UTC()
	r.LastAttemptAt = &attemptAt
	kind := "issue"
	switch {
	case r.Pending:
		kind = "deploy"
	case r.Resource != nil && !r.Force:
		kind = "renewal_check"
	case r.Resource != nil:
		kind = "renew"
	case r.Job.CertificateID != "":
		kind = "reissue"
	}
	r.Runs = append(r.Runs, Run{ID: domain.RandomID(), Kind: kind, StartedAt: m.now().UTC(), Status: "running", Entries: []LogEntry{}})
	if len(r.Runs) > maxRuns {
		r.Runs = r.Runs[len(r.Runs)-maxRuns:]
	}
	initialStage := "准备申请"
	if kind == "deploy" {
		initialStage = "准备部署"
	}
	appendEntry(r, m.now(), "info", initialStage, nil)
	r.Job.FailureCode, r.Job.FailureStage, r.Job.CleanupMessage = "", "", ""
	r.Job.Status = "running"
	r.Job.Message = initialStage
	r.Job.UpdatedAt = m.now()
	if err := m.save(r); err != nil {
		m.active = ""
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	checkpoint := func() error { m.mu.Lock(); defer m.mu.Unlock(); return m.save(r) }
	failureStage := initialStage
	progress := func(message string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		message = safeStage(message)
		if cleanupStage(message) {
			r.Job.CleanupMessage = message
		} else {
			failureStage = message
		}
		r.Job.Message, r.Job.UpdatedAt = message, m.now()
		level := "info"
		if message == "DNS TXT 清理失败，请检查服务商权限和残留记录" {
			level = "warning"
		}
		appendEntry(r, m.now(), level, message, nil)
		_ = m.save(r)
	}
	workCtx, cancel := context.WithTimeout(ctx, 35*time.Minute)
	defer cancel()
	workCtx = withLogSink(workCtx, func(entry LogEntry) {
		m.mu.Lock()
		defer m.mu.Unlock()
		appendDiagnostic(r, entry)
		_ = m.save(r)
	})
	var err error
	if r.Pending {
		if c, e := leaf(r.Resource); e != nil || !c.NotAfter.After(m.now()) {
			r.Pending = false
			r.Force = true
		}
	}
	if !r.Pending {
		err = m.issue(workCtx, r, checkpoint, progress)
	}
	if err == nil && workCtx.Err() != nil {
		err = workCtx.Err()
	}
	if err == nil && r.Pending {
		progress("部署证书")
		var id string
		targetID := r.Job.CertificateID
		if targetID == "" {
			targetID = r.Job.ID
		}
		id, err = m.deploy(targetID, r.Input.Name, r.Resource)
		if err == nil {
			r.Job.CertificateID = id
			r.Pending = false
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = ""
	r.Job.UpdatedAt = m.now()
	if err != nil {
		// Never publish raw provider responses: they may contain credentials or validation tokens.
		r.Job.Status = "failed"
		r.Job.Failures++
		stage := failureStage
		d := classifyError(err, stage)
		r.Job.FailureCode, r.Job.FailureStage = d.Code, d.Stage
		r.Job.Message = d.Stage + "：" + d.Error()
		appendEntry(r, m.now(), "error", d.Message, d)
		if r.Pending {
			r.Job.Message = "证书已签发，部署失败；旧证书保留，将重试部署"
		}
		delay := time.Minute * 5 * time.Duration(1<<min(r.Job.Failures-1, 9))
		if delay > 24*time.Hour {
			delay = 24 * time.Hour
		}
		r.Job.NextAttempt = m.now().Add(delay)
		if retryAt := m.now().Add(time.Duration(d.RetryAfterSeconds) * time.Second); retryAt.After(r.Job.NextAttempt) {
			r.Job.NextAttempt = retryAt
		}
	} else {
		r.Job.FailureCode, r.Job.FailureStage = "", ""
		r.Job.Status = "ready"
		r.Job.Message = "证书已保存，自动续期已启用"
		r.Job.Failures = 0
		r.Force = false
		if c, e := leaf(r.Resource); e == nil {
			r.Job.NotAfter = &c.NotAfter
			r.Job.NextAttempt = c.NotAfter.Add(-c.NotAfter.Sub(c.NotBefore) / 3)
		}
		if r.RenewAt != nil {
			r.Job.NextAttempt = *r.RenewAt
		}
		if r.Job.NextAttempt.After(m.now().Add(6 * time.Hour)) {
			r.Job.NextAttempt = m.now().Add(6 * time.Hour)
		}
		if !r.Job.NextAttempt.After(m.now()) {
			r.Job.NextAttempt = m.now().Add(time.Hour)
		}
	}
	run := &r.Runs[len(r.Runs)-1]
	finished := m.now().UTC()
	run.FinishedAt = &finished
	run.Status = r.Job.Status
	if err == nil {
		appendEntry(r, finished, "success", r.Job.Message, nil)
	}
	if e := m.save(r); e != nil {
		run.Status = "failed"
		appendEntry(r, finished, "error", "无法保存 ACME 任务状态", diagnostic("storage"))
		// Keep the successful in-memory result so a transient disk failure does not immediately reissue.
		r.Job.Status = "failed"
		r.Job.Message = "无法保存 ACME 任务状态，请检查磁盘空间与权限"
		m.records[r.Job.ID] = clone(r)
	}
}
