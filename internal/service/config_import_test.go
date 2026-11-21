package service

import (
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestImportPreviewCommitAndLocationConflict(t *testing.T) {
	s := testService(t)
	r := ProxyRule{ID: "abcdef123456", Name: "demo", Domains: []string{"demo.test"}, ListenPort: 19187, UpstreamHost: "127.0.0.1", UpstreamPort: 8123, Enabled: false}
	input := ConfigImport{Rules: []ProxyRule{r}}
	preview, err := s.ImportConfig(input, false)
	if err != nil || len(preview) != 1 || len(s.State().Rules) != 0 {
		t.Fatalf("preview: %v, %v", preview, err)
	}
	input.Expected = []string{"stale"}
	if _, err = s.ImportConfig(input, true); err == nil || len(s.State().Rules) != 0 {
		t.Fatal("stale preview was committed")
	}
	input.Expected = preview
	if _, err = s.ImportConfig(input, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportConfig(input, true); err == nil {
		t.Fatal("duplicate commit succeeded")
	}
	content, err := s.RuleConfig(r.ID, false)
	if err != nil || content != preview[0] || s.State().Rules[0].Enabled {
		t.Fatalf("disabled preview mismatch: %v", err)
	}
	loc := domain.LocationRule{ID: "123456abcdef", Name: "upload", Match: "prefix", Path: "/upload", Enabled: true, Settings: domain.LocationSettings{BackendType: "proxy", UpstreamHost: "127.0.0.1", UpstreamPort: 8181, ReadTimeoutSeconds: 300, Expires: "7d"}}
	appendPath := ConfigImport{Target: r.ID, Location: &loc}
	preview, err = s.ImportConfig(appendPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(preview[0], "proxy_connect_timeout 10s;") != 2 || strings.Count(preview[0], "expires 7d;") != 1 || strings.Count(preview[0], "proxy_read_timeout 300s;") != 1 {
		t.Fatal(preview[0])
	}
	appendPath.Expected = preview
	if _, err = s.ImportConfig(appendPath, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportConfig(appendPath, false); err == nil || len(s.State().Rules[0].Locations) != 1 {
		t.Fatal("duplicate location accepted")
	}
}

func TestSourceRuleReplacementIsAtomic(t *testing.T) {
	s := testService(t)
	const prefix = "# 保留文件注释\n"
	const block = "server { listen 19187; server_name demo.test; location / { proxy_pass http://127.0.0.1:8123; } }"
	const suffix = "\nserver { listen 19188; return 204; }\n"
	file := domain.CustomConfig{Name: "custom-sites.conf", Content: prefix + block + suffix}
	if _, err := s.SaveCustomConfig("", file); err != nil {
		t.Fatal(err)
	}
	r := ProxyRule{ID: "abcdef123456", Name: "demo", Domains: []string{"demo.test"}, ListenPort: 19187, UpstreamHost: "127.0.0.1", UpstreamPort: 8123, Enabled: true}
	input := ConfigImport{Rules: []ProxyRule{r}, Source: &ImportSource{Name: file.Name, Expected: file.Content, Start: len(prefix), End: len(prefix) + len(block)}}
	preview, err := s.ImportConfig(input, false)
	if err != nil {
		t.Fatal(err)
	}
	input.Expected = preview
	input.Source.Expected += " "
	if _, err := s.ImportConfig(input, true); err == nil || len(s.State().Rules) != 0 || s.State().CustomConfigs[0] != file {
		t.Fatal("stale source was converted")
	}
	input.Source.Expected = file.Content
	if _, err := s.ImportConfig(input, true); err != nil {
		t.Fatal(err)
	}
	if len(s.State().Rules) != 1 || s.State().CustomConfigs[0].Content != prefix+suffix {
		t.Fatal("conversion changed another block")
	}
}
