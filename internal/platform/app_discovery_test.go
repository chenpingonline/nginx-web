package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func discoveryFixture(t *testing.T, root, app, manifest, config string) string {
	t.Helper()
	base := filepath.Join(root, app)
	if err := os.MkdirAll(filepath.Join(base, "target", "ui"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "manifest"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "target", "ui", "config"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestDiscoverAppEntriesUsesDeclaredWebPortsAndNativeGateways(t *testing.T) {
	root := t.TempDir()
	discoveryFixture(t, root, "Music", "display_name=Music\nservice_port=4533", `{".url":{
	 "music.main":{"title":"音乐","type":"url","protocol":"http","port":"${TRIM_SERVICE_PORT}","url":"/"},
	 "music.hidden":{"type":"url","protocol":"http","port":"5555","noDisplay":true},
	 "music.external":{"type":"url","protocol":"https","port":"443","url":"https://example.com/"},
	 "music.cgi":{"type":"iframe","protocol":"http","port":"8080","url":"/cgi/ThirdParty/music/index.cgi/"}
	}}`)
	discoveryFixture(t, root, "Gitea", "desktop_uidir=ui", `{".url":{"gitea.main":{"title":"Gitea","type":"url","protocol":"http","port":"${port}","url":"/"}}}`)
	discoveryFixture(t, root, "Native", "", `{".url":{"native.main":{"title":"原生应用","type":"iframe","port":"7777","gatewayPrefix":"/app/native","gatewaySocket":"app.sock"}}}`)
	discoveryFixture(t, root, "TLS", "", `{".url":{"tls.main":{"type":"iframe","protocol":"https","port":8443}}}`)
	discoveryFixture(t, root, "nginx-web", "", `{".url":{"self":{"type":"iframe","protocol":"http","port":"8080"}}}`)
	got, err := DiscoverAppEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 4 || got.Unreadable != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Entries[0].App != "Gitea" || got.Entries[0].Target != "" || !strings.Contains(got.Entries[0].Note, "向导") {
		t.Fatalf("must not guess variable port: %+v", got.Entries[0])
	}
	if got.Entries[1].Target != "http://127.0.0.1:4533" {
		t.Fatalf("service variable not resolved: %+v", got.Entries[1])
	}
	if got.Entries[2].Target != "" || got.Entries[2].GatewayURL != "/app/native/" {
		t.Fatalf("native socket must not become TCP: %+v", got.Entries[2])
	}
	if got.Entries[3].Target != "https://127.0.0.1:8443" {
		t.Fatal(got.Entries[3])
	}
}

func TestDiscoveryRejectsInvalidPortsPathsAndFiles(t *testing.T) {
	root := t.TempDir()
	discoveryFixture(t, root, "bad-ui", "desktop_uidir=../../private", `{}`)
	discoveryFixture(t, root, "bad-json", "", `{".url":`)
	discoveryFixture(t, root, "invalid", "", `{".url":{
	 "zero":{"type":"url","port":"0"},"large":{"type":"url","port":"65536"},
	 "tcp":{"type":"url","protocol":"tcp","port":"3306"},
	 "native":{"type":"iframe","gatewayPrefix":"//example.com","gatewaySocket":"app.sock"},
	 "variable":{"type":"url","protocol":"http","port":"${port}","url":"${path}"}
	}}`)
	base := discoveryFixture(t, root, "escape", "", `{}`)
	out := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(out, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(base, "target/ui/config")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(base, "target/ui/config")); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverAppEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 0 || got.Unreadable != 3 {
		t.Fatalf("unexpected unsafe discovery: %+v", got)
	}
	if _, err = DiscoverAppEntries(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing registry must report error")
	}
	if _, err = readDiscoveryFile(out, 1); err == nil {
		t.Fatal("file size must be bounded")
	}
}

func TestDiscoveryAllowsFnosTargetSymlink(t *testing.T) {
	root := t.TempDir()
	base := discoveryFixture(t, root, "app", "", `{".url":{"app":{"type":"url","protocol":"http","port":"8011"}}}`)
	volume := filepath.Join(t.TempDir(), "installed")
	if err := os.Rename(filepath.Join(base, "target"), volume); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(volume, filepath.Join(base, "target")); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverAppEntries(root)
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("normal fnOS target symlink rejected: %+v %v", got, err)
	}
}

func TestDiscoveryFallsBackFromUnresolvedTitles(t *testing.T) {
	root := t.TempDir()
	config := `{".url":{"main":{"title":"${entry.title}","type":"iframe","gatewayPrefix":"/app/demo","gatewaySocket":"app.sock"}}}`
	discoveryFixture(t, root, "a", "display_name=应用商店", config)
	discoveryFixture(t, root, "b", "display_name=${title}", config)
	got, err := DiscoverAppEntries(root)
	if err != nil || len(got.Entries) != 2 || got.Entries[0].Name != "应用商店" || got.Entries[1].Name != "b" {
		t.Fatalf("unresolved title leaked: %+v %v", got, err)
	}
}
