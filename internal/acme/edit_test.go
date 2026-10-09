package acme

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certificate"
)

func TestEditPreservesMaskedSecretsAndPendingDeployment(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	in := validInput()
	in.DNSConfig = map[string]string{"CF_DNS_API_TOKEN": "dns-private-token", "CF_ZONE_API_TOKEN": "optional-zone-token", "CLOUDFLARE_TTL": "120"}
	j, err := m.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	r := clone(m.records[j.ID])
	r.Resource, r.Pending, r.Job.CertificateID, r.Job.Enabled = testResource(t, time.Now()), true, "existing-cert", false
	r.AccountKey = []byte("private-account-key")
	r.Registration = &legoacme.ExtendedAccount{Location: "https://ca.example/acct/42"}
	if err := m.save(r); err != nil {
		t.Fatal(err)
	}
	settings, _ := m.Settings(j.ID)
	public, _ := json.Marshal(settings)
	for _, secret := range []string{"dns-private-token", "optional-zone-token", "private-account-key", "acct/42"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("safe config leaked", secret)
		}
	}
	if len(settings.ConfiguredSecrets) != 2 || settings.DNSConfig["CLOUDFLARE_TTL"] != "120" {
		t.Fatal("masked view missing settings", settings)
	}
	settings.Name = "edited-task"
	settings.DNSConfig["CLOUDFLARE_TTL"] = "180"
	job, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision, ClearSecrets: []string{"dns_config.CF_ZONE_API_TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	stored := m.records[j.ID]
	if stored.Input.DNSConfig["CF_DNS_API_TOKEN"] != "dns-private-token" || stored.Input.DNSConfig["CF_ZONE_API_TOKEN"] != "" || stored.Input.DNSConfig["CLOUDFLARE_TTL"] != "180" {
		t.Fatal("secret merge/clear failed")
	}
	if job.Enabled || !job.NextAttempt.Equal(j.NextAttempt) || job.Revision != j.Revision+1 || job.CertificateID != "existing-cert" || stored.Resource == nil || !stored.Pending || stored.Registration == nil {
		t.Fatal("save changed scheduling or certificate/account")
	}
	before, _ := json.Marshal(stored)
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	settings, _ = m.Settings(j.ID)
	m.active = j.ID
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatal("active update accepted", err)
	}
	m.active = ""
	settings.Domains = nil
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	after, _ := json.Marshal(m.records[j.ID])
	if string(before) != string(after) {
		t.Fatal("rejected edit mutated stored configuration")
	}
	reloaded, err := New(m.dir, nil)
	if err != nil || reloaded.records[j.ID].Input.DNSConfig["CF_DNS_API_TOKEN"] != "dns-private-token" {
		t.Fatal("edit not durable", err)
	}
}

func TestEditLegacySecretsNewOrderAndStableCertificate(t *testing.T) {
	now := time.Now().UTC()
	m, _ := New(t.TempDir(), nil)
	m.now = func() time.Time { return now }
	j, _ := m.Create(validInput())
	r := clone(m.records[j.ID])
	r.Job.CertificateID = "old-certificate"
	r.Resource = testResource(t, now)
	r.AccountKey = []byte("keep-account")
	_ = m.save(r)
	now = now.Add(2 * time.Minute)
	settings, _ := m.Settings(j.ID)
	if settings.DNSConfig["CF_DNS_API_TOKEN"] != "" || len(settings.ConfiguredSecrets) != 1 {
		t.Fatal("legacy secret exposed/missing")
	}
	settings.Domains = []string{"new.example.com"}
	job, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision, Retry: true})
	if err != nil {
		t.Fatal(err)
	}
	stored := m.records[j.ID]
	if stored.Resource != nil || stored.Pending || string(stored.AccountKey) != "keep-account" || stored.Input.DNSConfig["CF_DNS_API_TOKEN"] != "secret-token-test" || !job.Enabled || job.Status != "queued" {
		t.Fatal("new order did not retain account/config")
	}
	deployments := 0
	m.issue = func(context.Context, *Record, func() error, func(string)) error {
		return errors.New("invalid credentials: secret-token-test")
	}
	m.deploy = func(id, name string, r *certificate.Resource) (string, error) { deployments++; return id, nil }
	m.step(context.Background())
	if deployments != 0 || m.List()[0].CertificateID != "old-certificate" {
		t.Fatal("failed replacement touched deployed certificate")
	}
	now = now.Add(2 * time.Minute)
	settings, _ = m.Settings(j.ID)
	settings.DNSConfig["CF_DNS_API_TOKEN"] = "replacement-secret"
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision, Retry: true}); err != nil {
		t.Fatal(err)
	}
	m.issue = func(_ context.Context, r *Record, save func() error, progress func(string)) error {
		r.Resource, r.Pending = testResource(t, now), true
		return save()
	}
	m.step(context.Background())
	if deployments != 1 || m.List()[0].CertificateID != "old-certificate" {
		t.Fatal("successful replacement changed reference")
	}
}

func TestEditCARequiresNewEABAndResetsAccount(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	in := validInput()
	in.CA = "custom"
	in.DirectoryURL = "https://first.example/directory"
	in.Credentials.EABKID = "private-kid"
	in.Credentials.EABHMAC = base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	j, err := m.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	r := clone(m.records[j.ID])
	r.AccountKey, r.Registration = []byte("old-account"), &legoacme.ExtendedAccount{Location: "https://first.example/acct/42"}
	_ = m.save(r)
	settings, _ := m.Settings(j.ID)
	if settings.Credentials.EABKID != "" || settings.Credentials.EABHMAC != "" {
		t.Fatal("EAB exposed")
	}
	settings.Name = "safe-edit"
	j, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision})
	if err != nil || m.records[j.ID].Registration == nil || m.records[j.ID].Input.Credentials.EABKID != "private-kid" {
		t.Fatal("EAB/account not preserved", err)
	}
	settings, _ = m.Settings(j.ID)
	settings.CA = "zerossl"
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); err == nil {
		t.Fatal("reused old CA EAB")
	}
	settings.Credentials.EABKID, settings.Credentials.EABHMAC = "new-kid", in.Credentials.EABHMAC
	if _, err = m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); err != nil {
		t.Fatal(err)
	}
	if m.records[j.ID].Registration != nil || len(m.records[j.ID].AccountKey) != 0 {
		t.Fatal("old CA account retained")
	}
}

func TestLogsAreDurableBoundedAndContainOnlySafeDiagnostics(t *testing.T) {
	now := time.Now().UTC()
	m, _ := New(t.TempDir(), nil)
	m.now = func() time.Time { return now }
	j, _ := m.Create(validInput())
	calls := 0
	m.issue = func(_ context.Context, _ *Record, _ func() error, progress func(string)) error {
		calls++
		progress("初始化 DNS 服务商")
		progress("token=secret-token-test https://user:password@host/?signature=private")
		if calls == 23 {
			for range 205 {
				progress("创建 DNS TXT 记录")
			}
		}
		return errors.New("InvalidToken secret-token-test private-key")
	}
	for range 23 {
		m.step(context.Background())
		now = now.Add(25 * time.Hour)
	}
	runs, err := m.Logs(j.ID)
	if err != nil || len(runs) != maxRuns {
		t.Fatal("unbounded history", len(runs), err)
	}
	for _, run := range runs {
		if len(run.Entries) > maxEntries || run.Status != "failed" || run.FinishedAt == nil || run.Entries[len(run.Entries)-1].Code != "dns_credentials" || run.Entries[0].Message != "准备申请" {
			t.Fatal("missing bounded outcome")
		}
	}
	if len(runs[len(runs)-1].Entries) != maxEntries {
		t.Fatal("entry limit not exercised")
	}
	public, _ := json.Marshal(runs)
	for _, value := range []string{"secret-token-test", "password", "signature", "private-key"} {
		if strings.Contains(string(public), value) {
			t.Fatal("log leaked", value)
		}
	}
	reloaded, err := New(m.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	persisted, _ := reloaded.Logs(j.ID)
	serialized, _ := json.Marshal(persisted)
	if string(public) != string(serialized) {
		t.Fatal("logs not durable")
	}
	persisted[0].Entries[0].Message = "mutated"
	current, _ := reloaded.Logs(j.ID)
	if current[0].Entries[0].Message == "mutated" {
		t.Fatal("API mutated private history")
	}
	info, _ := os.Stat(filepath.Join(m.dir, j.ID+".json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("private history permissions")
	}
}

func TestInterruptedHistoryMigration(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	j, _ := m.Create(validInput())
	r := clone(m.records[j.ID])
	r.Job.Status, r.Job.Revision = "running", 0
	r.Runs = []Run{{ID: "last-run", Kind: "issue", StartedAt: time.Now(), Status: "running", Entries: []LogEntry{}}}
	_ = m.save(r)
	reloaded, err := New(m.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := reloaded.Logs(j.ID)
	if reloaded.List()[0].Revision != 1 || reloaded.List()[0].Status != "queued" || runs[0].Status != "interrupted" || runs[0].FinishedAt == nil {
		t.Fatal("interrupted migration failed")
	}
	again, _ := New(m.dir, nil)
	runs, _ = again.Logs(j.ID)
	if len(runs[0].Entries) != 1 {
		t.Fatal("recovery event duplicated")
	}
}

func TestWorkerReturnsSafeNetworkDiagnosticAndPreservesParentHistory(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	in := validInput()
	in.CA, in.DirectoryURL = "custom", "https://127.0.0.1:1/directory"
	in.DNSConfig = map[string]string{"CF_DNS_API_TOKEN": "secret-never-logged"}
	j, err := m.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	m.step(context.Background())
	runs, _ := m.Logs(j.ID)
	last := runs[0].Entries[len(runs[0].Entries)-1]
	if last.Code != "network" || len(runs[0].Entries) < 3 {
		t.Fatal("worker lost details/history", runs)
	}
	public, _ := json.Marshal(runs)
	if strings.Contains(string(public), "secret-never-logged") {
		t.Fatal("worker secret exposed")
	}
}

func TestSavingConfigurationDoesNotExtendRetryCooldown(t *testing.T) {
	now := time.Now().UTC()
	m, _ := New(t.TempDir(), nil)
	m.now = func() time.Time { return now }
	j, _ := m.Create(validInput())
	m.issue = func(context.Context, *Record, func() error, func(string)) error {
		return errors.New("invalid credentials")
	}
	m.step(context.Background())
	settings, _ := m.Settings(j.ID)
	if _, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision, Retry: true}); err == nil {
		t.Fatal("immediate repeated execution accepted")
	}
	now = now.Add(2 * time.Minute)
	settings.Name = "save-before-retry"
	if _, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); err != nil {
		t.Fatal(err)
	}
	settings, _ = m.Settings(j.ID)
	if _, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision, Retry: true}); err != nil {
		t.Fatal("saving extended attempt cooldown", err)
	}
}
