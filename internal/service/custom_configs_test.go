package service

import (
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestCustomConfigCRUD(t *testing.T) {
	s := testService(t)
	created, err := s.SaveCustomConfig("", domain.CustomConfig{Name: "custom-app.conf", Content: "server { return 200; }"})
	if err != nil || created.Content[len(created.Content)-1:] != "\n" {
		t.Fatal(created, err)
	}
	updated, err := s.SaveCustomConfig(created.Name, domain.CustomConfig{Name: "custom-renamed.conf", Content: "server { return 204; }"})
	if err != nil || len(s.State().CustomConfigs) != 1 || s.State().CustomConfigs[0].Name != updated.Name || !s.State().Dirty {
		t.Fatal(updated, err)
	}
	if err = s.DeleteCustomConfig(updated.Name); err != nil || len(s.State().CustomConfigs) != 0 {
		t.Fatal(err)
	}
	if err = s.DeleteCustomConfig(updated.Name); err == nil {
		t.Fatal("deleted missing custom config")
	}
}

func TestCustomConfigRejectsStaleParameterEdit(t *testing.T) {
	s := testService(t)
	original, err := s.SaveCustomConfig("", domain.CustomConfig{Name: "custom-test.conf", Content: "server { listen 9090; }"})
	if err != nil {
		t.Fatal(err)
	}
	edit := domain.CustomConfig{Name: original.Name, Content: "server { listen 9091; }"}
	saved, err := s.UpdateCustomConfig(original.Name, edit, &original.Content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateCustomConfig(original.Name, original, &original.Content); err == nil || s.State().CustomConfigs[0] != saved {
		t.Fatal("stale edit must leave the newer draft intact")
	}
}
