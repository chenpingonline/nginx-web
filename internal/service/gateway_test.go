package service

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/platform"
)

func TestDecorateGatewayAppsAvoidsDuplicatesAndPathConflicts(t *testing.T) {
	result := platform.AppDiscovery{Entries: []platform.AppEntry{
		{App: "Gitea", Name: "Gitea"},
		{App: "Music", Target: "http://127.0.0.1:4533/"},
		{App: "A.B"}, {App: "A-B"}, {App: "原生应用", GatewayURL: "/app/native/"},
	}}
	routes := []domain.GatewayRoute{{Name: "Gitea", Path: "git", Target: "http://127.0.0.1:8418"}, {Name: "音乐", Path: "music", Target: "http://127.0.0.1:4533"}, {Path: "a-b"}}
	decorateGatewayApps(&result, routes)
	if !result.Entries[0].Existing || result.Entries[0].Target != "http://127.0.0.1:8418" || result.Entries[0].Path != "git" {
		t.Fatal(result.Entries[0])
	}
	if !result.Entries[1].Existing {
		t.Fatal("same target was not recognized")
	}
	if result.Entries[2].Path != "a-b-2" || result.Entries[3].Path != "a-b-3" {
		t.Fatal("slug collisions", result.Entries)
	}
	if result.Entries[4].Path != "" {
		t.Fatal("native gateway should not be re-proxied")
	}
}

func TestGatewayDraftValidationAndBackupRoundTrip(t *testing.T) {
	svc := testService(t)
	route, err := svc.SaveGatewayRoute("", domain.GatewayRoute{Name: " Music ", Path: "/MUSIC/", Target: "http://127.0.0.1:4533/", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if route.Path != "music" || route.Name != "Music" || route.Target != "http://127.0.0.1:4533" {
		t.Fatalf("normalization: %+v", route)
	}
	if _, ok := svc.GatewayRoute("music"); ok {
		t.Fatal("draft route activated")
	}
	if _, err := svc.SaveGatewayRoute("", route); err == nil {
		t.Fatal("duplicate path saved")
	}
	if len(svc.State().GatewayRoutes) != 1 {
		t.Fatal("invalid update changed state")
	}
	backup, err := svc.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.State.GatewayRoutes) != 1 {
		t.Fatal("route missing from backup")
	}
	restored := testService(t)
	state, err := restored.RestoreBackup(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || len(state.GatewayRoutes) != 1 {
		t.Fatal("restore lost gateway draft")
	}
	if _, ok := restored.GatewayRoute("music"); ok {
		t.Fatal("restore activated gateway draft")
	}
	// Simulate an applied configuration, then exercise draft discard.
	applied := svc.State()
	now := time.Now().UTC()
	applied.LastAppliedAt = &now
	applied.Dirty = false
	if err := svc.store.Replace(applied); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(applied)
	if err := os.WriteFile(svc.paths.AppliedState(), data, 0600); err != nil {
		t.Fatal(err)
	}
	svc.setAppliedGatewayRoutes(applied.GatewayRoutes)
	if err := svc.DeleteGatewayRoute(route.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DiscardDraft(); err != nil {
		t.Fatal(err)
	}
	if len(svc.State().GatewayRoutes) != 1 {
		t.Fatal("discard lost applied route")
	}
	if _, ok := svc.GatewayRoute("music"); !ok {
		t.Fatal("discard stopped applied route")
	}
}
