package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameConflictSourcesFollowIncludesAndListenAddresses(t *testing.T) {
	dir := t.TempDir()
	confD := filepath.Join(dir, "conf.d")
	if err := os.Mkdir(confD, 0750); err != nil {
		t.Fatal(err)
	}
	master := filepath.Join(dir, "nginx.conf")
	for path, content := range map[string]string{
		master: "events {} http { include conf.d/*.conf; }",
		filepath.Join(confD, "010-19653-abcdef123456.conf"): "server { listen 19653; server_name EXAMPLE.test; }",
		filepath.Join(confD, "custom-b.conf"):               "server { listen 19653; server_name example.test unique.test; }\nserver { listen 19653; server_name example.test; }",
		filepath.Join(confD, "custom-a.conf"):               "server { listen [::]:19653; server_name example.test; }\ninclude extra.inc;",
		filepath.Join(dir, "extra.inc"):                     "server { listen 19653; server_name example.test; }",
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Paths{NginxMaster: master, NginxConfD: confD})
	rules := []ProxyRule{{ID: "abcdef123456", Name: "验收规则"}}
	line := "nginx: [warn] conflicting server name \"example.test\" on 0.0.0.0:19653, ignored\n"
	warnings := m.nameWarnings(strings.Repeat(line, 3), rules)
	if len(warnings) != 3 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for i, loc := range []string{"extra.inc」第 1 行", "custom-b.conf」第 1 行", "custom-b.conf」第 2 行"} {
		if !strings.Contains(warnings[i], "使用规则「验收规则」") || !strings.Contains(warnings[i], loc+"的这项域名匹配被忽略") {
			t.Errorf("wrong source: %s", warnings[i])
		}
	}
	if ws := m.nameWarnings("syntax is ok", rules); len(ws) != 0 {
		t.Fatal(ws)
	}
	if ws := m.nameWarnings(line, rules); len(ws) != 1 || !strings.Contains(ws[0], "无法确认") {
		t.Fatal(ws)
	}
}

func TestNameConflictBetweenCustomServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-multiple.conf")
	content := "events {} http {\nserver {listen [::]:9090; server_name .abc.test;}\nserver {listen [::]:9090; server_name abc.test;}\nserver {listen 9090; server_name abc.test;}\n}"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(Paths{NginxMaster: path, NginxConfD: dir})
	ws := m.nameWarnings("nginx: [warn] conflicting server name \"abc.test\" on [::]:9090, ignored", nil)
	if len(ws) != 1 || !strings.Contains(ws[0], "使用「custom-multiple.conf」第 2 行。「custom-multiple.conf」第 3 行的这项域名匹配被忽略") {
		t.Fatal(ws)
	}
}

func TestNameConflictIdentifiesSelectedServerOnSharedLine(t *testing.T) {
	dir := t.TempDir()
	master := filepath.Join(dir, "nginx.conf")
	const file = "custom-three.conf"
	if err := os.WriteFile(master, []byte("events {} http { include "+file+"; }"), 0600); err != nil {
		t.Fatal(err)
	}
	content := "server { listen 19990; server_name independent.test; } server { listen 19990; server_name dup.test; } server { listen 19990; server_name dup.test unique.test; }"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ws := diagnoseNames("nginx: [warn] conflicting server name \"dup.test\" on 0.0.0.0:19990, ignored", master, dir, nil)
	if len(ws) != 1 {
		t.Fatal(ws)
	}
	w, i := ws[0].winner, ws[0].ignored
	if w.file != file || i.file != file || w.line != 1 || i.line != 1 || w.occurrence != 2 || i.occurrence != 3 {
		t.Fatalf("wrong blocks: %+v", ws)
	}
}
