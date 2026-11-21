package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/platform"
)

func TestDiscoveredGatewayMode(t *testing.T) {
	var prefix = "/app/nginx-web/proxy/demo/"
	for _, tc := range []struct{ name, body, mode string }{
		{"relative assets", "<script src='./ui.js'></script>", "native"},
		{"explicit prefix", "<base href='" + prefix + "'><img src='logo.png'>", "native"},
		{"root resource", "<script src='./ui.js'></script><form action='/login'>", "subpath"},
		{"external only", "<script src='https://cdn.example/ui.js'></script>", "subpath"},
		{"escaping relative path", "<link href='../style.css'>", "subpath"},
		{"no clue", "<h1>OK</h1>", "subpath"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Forwarded-Prefix") != prefix[:len(prefix)-1] {
					t.Error("missing prefix")
				}
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			entries := []platform.AppEntry{{Target: server.URL, Path: "demo"}, {}, {Existing: true}, {GatewayURL: "/app/demo/"}}
			recommendGatewayModes(context.Background(), entries)
			if entries[0].RecommendedMode != tc.mode || entries[1].RecommendedMode != "subpath" || entries[2].RecommendedMode != "" || entries[3].RecommendedMode != "" {
				t.Fatal(entries)
			}
		})
	}
}
