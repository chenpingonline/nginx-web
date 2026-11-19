package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func syncedDraftService(t *testing.T) (*AppService, State) {
	t.Helper()
	s := testService(t)
	base := migrationState()
	domain.PauseUnsupportedPorts(&base)
	base.PortMigrationPending = false
	base.RuntimeConfigVersion = runtimeConfigVersion
	base.Dirty = false
	writeMigrationJSON(t, s.paths.AppliedState(), base)
	if err := s.store.Replace(base); err != nil {
		t.Fatal(err)
	}
	return s, s.State()
}

func TestDraftContentReconciliation(t *testing.T) {
	s, base := syncedDraftService(t)
	if err := s.UpdateSettings(base.Settings); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty || s.store.Snapshot().Dirty || s.Overview().Dirty {
		t.Fatal("unchanged save marked pending")
	}
	edited := base.Settings
	edited.WorkerConnections++
	if err := s.UpdateSettings(edited); err != nil {
		t.Fatal(err)
	}
	if !s.State().Dirty {
		t.Fatal("real change lost")
	}
	changes := s.DraftChanges()
	if changes.Status != "pending" || len(changes.Changes) != 1 || changes.Changes[0].Section != "settings" || len(changes.Changes[0].Fields) != 1 || changes.Changes[0].Fields[0].Path != "worker_connections" {
		t.Fatalf("unexpected diff: %+v", changes)
	}
	if err := s.UpdateSettings(base.Settings); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty || s.store.Snapshot().Dirty || s.DraftChanges().Status != "synced" {
		t.Fatal("reverting value left a pending flag")
	}
	// Old flags and global certificate updates must not look like routing edits.
	if err := s.store.Update(func(state *State) error {
		state.Dirty = true
		state.Certificates = append(state.Certificates, domain.CertificateMeta{ID: "abcdef123459", Name: "renewed"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty {
		t.Fatal("stale flag or certificate inventory caused pending state")
	}
	restarted, err := New(s.paths)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.State().Dirty {
		t.Fatal("restart reintroduced stale pending state")
	}
}

func TestDraftBaselineMustMatchActivation(t *testing.T) {
	s, base := syncedDraftService(t)
	if err := s.saveRevision(base, "matching activation"); err != nil {
		t.Fatal(err)
	}
	other := domain.CloneState(base)
	later := base.LastAppliedAt.Add(time.Minute)
	other.LastAppliedAt = &later
	other.Settings.WorkerConnections++
	if err := s.saveRevision(other, "unrelated newer history"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.paths.AppliedState()); err != nil {
		t.Fatal(err)
	}
	if s.DraftChanges().Status != "synced" {
		t.Fatal("did not find matching history baseline")
	}
	revisions, _ := s.ListRevisions()
	for _, revision := range revisions {
		if revision.State.LastAppliedAt.Equal(*base.LastAppliedAt) {
			if err := s.DeleteRevision(revision.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if s.Overview().AppliedKnown || s.DraftChanges().Status != "unknown" {
		t.Fatal("unrelated history or a clean flag accepted as evidence of activation")
	}
}

func TestDraftDiffIncludesOnlyChangedEntities(t *testing.T) {
	s, base := syncedDraftService(t)
	if err := s.store.Update(func(state *State) error {
		state.Rules[1].UpstreamPort++
		state.Rules = state.Rules[1:] // one deletion and one change; no unchanged stream
		state.GatewayRoutes = []domain.GatewayRoute{{ID: "abcdef123459", Name: "new gateway", Path: "test", Target: "http://127.0.0.1:8080", Enabled: true}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	changes := s.DraftChanges()
	if changes.Status != "pending" || len(changes.Changes) != 3 {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	counts := map[string]int{}
	for _, change := range changes.Changes {
		counts[change.Operation]++
		if change.Section == "stream_rules" {
			t.Fatal("unchanged rules leaked into diff")
		}
		if change.Operation == "modified" && (len(change.Fields) != 1 || change.Fields[0].Path != "upstream_port") {
			t.Fatalf("unchanged fields included: %+v", change)
		}
	}
	if counts["added"] != 1 || counts["deleted"] != 1 || counts["modified"] != 1 {
		t.Fatal(counts)
	}
	// Restoring the same applied version is synchronized, not a new draft.
	if err := s.saveRevision(base, "baseline"); err != nil {
		t.Fatal(err)
	}
	revisions, _ := s.ListRevisions()
	if _, err := s.RestoreRevision(revisions[0].ID); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty || s.State().DraftRevisionID != "" {
		t.Fatal("restoring identical content marked pending")
	}
}

func TestDraftDiffNeverExposesSecrets(t *testing.T) {
	fields := changedFields("auth_profiles", "users", []any{map[string]any{"password_hash": "old-private-hash"}}, []any{map[string]any{"password_hash": "new-private-hash"}})
	fields = append(fields, changedFields("custom_configs", "content", "old-private-config", "new-private-config")...)
	fields = append(fields, changedFields("gateway_routes", "target", "http://alice:private-pass@example.com/a?token=private-query", "http://bob:new-private-pass@example.com/b?token=new-private-query")...)
	raw, _ := json.Marshal(fields)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "token=") {
		t.Fatalf("secret leaked: %s", raw)
	}
	if len(fields) != 3 || !fields[0].Sensitive || !fields[1].Sensitive {
		t.Fatal("secret changes not identified")
	}
}

func TestConfigurationOrderAndEditTimes(t *testing.T) {
	s, base := syncedDraftService(t)
	if err := s.store.Update(func(state *State) error { state.Rules[0].UpdatedAt = time.Now(); return nil }); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty || len(s.DraftChanges().Changes) != 0 {
		t.Fatal("edit timestamp counted as configuration")
	}
	if err := s.store.Update(func(state *State) error { state.Rules[0], state.Rules[1] = state.Rules[1], state.Rules[0]; return nil }); err != nil {
		t.Fatal(err)
	}
	if !s.State().Dirty || len(s.DraftChanges().Changes) != 1 || s.DraftChanges().Changes[0].Name != "排列顺序" {
		t.Fatal("routing order lost")
	}
	if sameToggleConfiguration(base, s.State()) {
		t.Fatal("order compared as an unordered set")
	}
}

func TestActivationRecordsCandidateAndPreservesLaterDraft(t *testing.T) {
	s, candidate := syncedDraftService(t)
	later := candidate.Settings
	later.WorkerConnections++
	if err := s.UpdateSettings(later); err != nil {
		t.Fatal(err)
	}
	if _, err := s.recordAppliedState(candidate, ApplyResult{Message: "applied"}, "activation"); err != nil {
		t.Fatal(err)
	}
	current := s.State()
	applied, known := s.appliedState(current)
	if !known || !current.Dirty || applied.Settings.WorkerConnections != candidate.Settings.WorkerConnections || current.Settings.WorkerConnections != later.WorkerConnections {
		t.Fatal("later draft was recorded as active or lost")
	}
	revisions, _ := s.ListRevisions()
	if len(revisions) != 1 || revisions[0].State.Settings.WorkerConnections != candidate.Settings.WorkerConnections {
		t.Fatal("history captured a newer draft instead of the applied candidate")
	}
	if _, err := s.recordAppliedState(current, ApplyResult{Message: "applied"}, "activation"); err != nil {
		t.Fatal(err)
	}
	if s.State().Dirty {
		t.Fatal("successful activation failed to synchronize")
	}
}

func TestActivationSnapshotFailureReportsUnknown(t *testing.T) {
	s, candidate := syncedDraftService(t)
	if err := os.Remove(s.paths.AppliedState()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.paths.AppliedState(), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.recordAppliedState(candidate, ApplyResult{Message: "applied"}, "activation"); err == nil {
		t.Fatal("snapshot persistence failure hidden")
	}
	if s.DraftChanges().Status != "unknown" || s.Overview().AppliedKnown || s.State().LastApplyError == "" {
		t.Fatal("failed snapshot persistence reported synchronized")
	}
}

func TestDraftBaselineRejectsConflictingSnapshot(t *testing.T) {
	s, base := syncedDraftService(t)
	if err := s.saveRevision(base, "old activation"); err != nil {
		t.Fatal(err)
	}
	conflicting := domain.CloneState(base)
	later := base.LastAppliedAt.Add(time.Minute)
	conflicting.LastAppliedAt = &later
	conflicting.Settings.WorkerConnections++
	writeMigrationJSON(t, s.paths.AppliedState(), conflicting)
	if s.Overview().AppliedKnown || s.DraftChanges().Status != "unknown" {
		t.Fatal("older history masked a conflicting activation snapshot")
	}
}
