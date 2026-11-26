package service

import (
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestImportPreviewCommitAndLocationConflict(t *testing.T) {
	s := testService(t)
	r := ProxyRule{ID: "abcdef123456", Name: "demo", Domains: []string{"demo.test"}, ListenPort: 19187, UpstreamHost: "127.0.0.1", UpstreamPort: 8123, ClientMaxBodySize: "512k", Enabled: false}
	input := ConfigImport{Rules: []ProxyRule{r}}
	preview, err := s.ImportConfig(input, false)
	if err != nil || len(preview) != 1 || len(s.State().Rules) != 0 {
		t.Fatalf("preview: %v, %v", preview, err)
	}
	if !strings.Contains(preview[0], "client_max_body_size 512k;") {
		t.Fatal(preview[0])
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

func TestImportSiteAndPoolInOneDraft(t *testing.T) {
	s := testService(t)
	pool := domain.UpstreamPool{ID: "abcdef123456", Name: "app", Protocol: "http", Servers: []domain.UpstreamServer{{Host: "127.0.0.1", Port: 9010}}}
	rule := ProxyRule{ID: "abcdef123457", Name: "site", Domains: []string{"import.test"}, ListenPort: 19400, UpstreamPoolID: pool.ID, Enabled: true}
	input := ConfigImport{Pools: []domain.UpstreamPool{pool}, Rules: []ProxyRule{rule}}
	preview, err := s.ImportConfig(input, false)
	if err != nil || len(preview) != 2 || !strings.Contains(preview[0], "upstream fnproxy_up_"+pool.ID) || !strings.Contains(preview[1], "proxy_pass http://fnproxy_up_"+pool.ID) {
		t.Fatalf("preview: %v %v", preview, err)
	}
	input.Expected = preview
	input.Pools[0].Servers[0].Port++
	if _, err := s.ImportConfig(input, true); err == nil || len(s.State().Rules) != 0 || len(s.State().UpstreamPools) != 0 {
		t.Fatal("stale preview partially committed")
	}
	input.Pools[0].Servers[0].Port--
	if _, err := s.ImportConfig(input, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportConfig(input, true); err == nil || len(s.State().Rules) != 1 || len(s.State().UpstreamPools) != 1 {
		t.Fatal("duplicate commit")
	}
}

func TestAutoImportStreamBatch(t *testing.T) {
	s := testService(t)
	omissions := &domain.StreamOmissions{ConnectTimeout: true, ProxyTimeout: true, AccessLog: true, UDPReusePort: true}
	tcp := domain.StreamRule{ID: "ab1234567890", Name: "tcp", Protocol: "tcp", Enabled: true, ListenPort: 19885, UpstreamHost: "127.0.0.1", UpstreamPort: 19881, ConnectTimeoutSeconds: 60, ProxyTimeoutSeconds: 600, Omissions: omissions}
	udp := tcp
	udp.ID, udp.Name, udp.Protocol = "ab1234567891", "udp", "udp"
	input := ConfigImport{NewFileName: "custom-two.stream", Streams: []domain.StreamRule{tcp, udp}}
	preview, err := s.ImportConfig(input, false)
	if err != nil || len(preview) != 2 || len(s.State().StreamRules) != 0 {
		t.Fatalf("preview %v, %v", preview, err)
	}
	for _, text := range preview {
		for _, extra := range []string{"proxy_connect_timeout", "proxy_timeout", "access_log", "reuseport"} {
			if strings.Contains(text, extra) {
				t.Fatalf("added %s: %s", extra, text)
			}
		}
	}
	input.Expected = preview
	input.Streams[1].UpstreamPort++
	if _, err := s.ImportConfig(input, true); err == nil || len(s.State().StreamRules) != 0 {
		t.Fatal("committed a changed batch")
	}
	input.Streams[1].UpstreamPort--
	if _, err := s.ImportConfig(input, true); err != nil || len(s.State().StreamRules) != 2 {
		t.Fatalf("commit: %v", err)
	}
}

func TestCommentedSiteAndNamedPoolRemainLinkedAfterEdit(t *testing.T) {
	s := testService(t)
	pool := domain.UpstreamPool{ID: "0123456789ab", Name: "web", NginxName: "web", Protocol: "http", Servers: []domain.UpstreamServer{{Host: "127.0.0.1", Port: 9980}},
		Comments: []domain.ConfigComment{{Anchor: "/upstream:0/server:0", Text: "# 主节点", Inline: true}}}
	site := ProxyRule{ID: "abcdef123456", Name: "site", Domains: []string{"pool.test"}, ListenPort: 19196, UpstreamPoolID: pool.ID, Enabled: true,
		Comments: []domain.ConfigComment{{Anchor: "/server:0/listen:0", Text: "# 入口", Inline: true}, {Anchor: "/server:0/}", Text: "# 尾注", After: true}}}
	input := ConfigImport{NewFileName: "custom-pool.conf", Pools: []domain.UpstreamPool{pool}, Rules: []ProxyRule{site}}
	preview, err := s.ImportConfig(input, false)
	if err != nil || len(preview) != 2 || !strings.Contains(preview[0], "upstream web {") || !strings.Contains(preview[0], "; # 主节点") || !strings.Contains(preview[1], "proxy_pass http://web;") || !strings.HasSuffix(preview[1], "}\n# 尾注\n") {
		t.Fatalf("preview %v: %v", err, preview)
	}
	input.Expected = preview
	if _, err = s.ImportConfig(input, true); err != nil {
		t.Fatal(err)
	}
	pool.Name, pool.NginxName, pool.Comments = "新名称", "", nil
	if _, err = s.UpdateUpstreamPool(pool.ID, pool); err != nil {
		t.Fatal(err)
	}
	site.ListenPort, site.Comments = 19197, nil
	if _, err = s.UpdateRule(site.ID, site); err != nil {
		t.Fatal(err)
	}
	text, err := s.RuleConfig(site.ID, false)
	if err != nil || !strings.Contains(text, "19197; # 入口") || !strings.Contains(text, "proxy_pass http://web;") || !strings.Contains(text, "# 尾注") {
		t.Fatalf("updated site %v %s", err, text)
	}
	if got := s.State().UpstreamPools[0]; got.ConfigName() != "web" || len(got.Comments) != 1 {
		t.Fatalf("updated pool: %#v", got)
	}
}
