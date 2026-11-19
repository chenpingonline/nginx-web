package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/gatewayclient"
)

func TestGatewayProbeAuthorizationAndNoMutation(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer upstream.Close()
	api, svc, _ := gatewayTestAPI(t, "http://127.0.0.1:8080")
	before := svc.State()
	for _, admin := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "/api/gateway-probe", strings.NewReader(`{"target":"`+upstream.URL+`"}`))
		request.Header.Set("X-FnProxy-Request", "1")
		if admin {
			request.Header.Set("X-Trim-Isadmin", "true")
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if !admin {
			if response.Code != 403 {
				t.Fatal(response.Code)
			}
			continue
		}
		var result gatewayclient.ProbeResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Code != "certificate_untrusted" || !result.CanTrust {
			t.Fatal(response.Body.String())
		}
	}
	if !reflect.DeepEqual(before, svc.State()) {
		t.Fatal("probe mutated draft or applied state")
	}
}
