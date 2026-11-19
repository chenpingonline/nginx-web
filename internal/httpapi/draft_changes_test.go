package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	webassets "github.com/chenpingonline/nginx-web/web"
)

func TestDraftChangesRequiresAdministrator(t *testing.T) {
	t.Setenv("FNPROXY_DEV_ALLOW", "0")
	api := New(nil, webassets.Assets)
	for _, path := range []string{"/api/draft-changes", gatewayPrefix + "/api/draft-changes"} {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("configuration diff exposed without administrator access: %s returned %d", path, response.Code)
		}
	}
}
