package nginx

import (
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestMenuSwitchesKeepLoggingDestinationsIndependent(t *testing.T) {
	m := New(Paths{NginxAccessLog: "/access.log", NginxLogDir: "/logs"})
	for _, menu := range []string{"errors", "logs", "topology", "rules"} {
		t.Run(menu, func(t *testing.T) {
			state := domain.DefaultState()
			state.Settings.HiddenMenus = []string{menu}
			master, files, err := m.render(state, "/conf")
			if err != nil {
				t.Fatal(err)
			}
			for id, destination := range map[string]string{"logs": "access_log \"/access.log\"", "errors": " fnproxy_metrics buffer=", "topology": " fnproxy_topology;"} {
				if strings.Contains(master, destination) != (menu != id) {
					t.Errorf("unexpected %s log destination when %s is hidden", id, menu)
				}
			}
			_, monitors := files["000-monitoring.conf"]
			if monitors != (menu != "errors") {
				t.Error("unexpected monitoring listener")
			}
			listens := false
			for name := range files {
				if strings.HasPrefix(name, "000-default-") {
					listens = true
				}
			}
			if listens != (menu != "rules") {
				t.Error("unexpected default HTTP listener")
			}
		})
	}
}
