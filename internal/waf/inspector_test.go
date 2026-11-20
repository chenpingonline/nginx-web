package waf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Nginx buffers uploads before inspection. A known oversized body should take
// the configured action without running content rules on a truncated document.
func TestOversizeBodyPolicy(t *testing.T) {
	inspector := new(Inspector)
	body := strings.Repeat("x", requestBodyLimit+1)
	for _, tc := range []struct {
		mode   string
		status int
	}{{"detect", http.StatusOK}, {"block", http.StatusRequestEntityTooLarge}} {
		req := httptest.NewRequest("POST", "http://files.example.net/upload", strings.NewReader(body))
		req.Header.Set("X-Fnproxy-Mode", tc.mode)
		req.Header.Set("X-Fnproxy-Instance", "123456abcdef")
		req.Header.Set("X-Fnproxy-Site", "abcdef123456")
		req.Header.Set("X-Fnproxy-Target", "0")
		req.Header.Set("X-Fnproxy-Protocol", "HTTP/1.1")
		req.Header.Set("X-Fnproxy-Client", "192.0.2.1")
		req.Header.Set("User-Agent", "nginx-web-test")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		inspector.ServeHTTP(w, req)
		if w.Code != tc.status || (w.Header().Get("X-Accel-Redirect") != "") != (tc.mode == "detect") {
			t.Fatalf("%s returned %d, headers %v", tc.mode, w.Code, w.Header())
		}
		if remaining, _ := io.Copy(io.Discard, req.Body); remaining != 0 {
			t.Fatalf("inspection copy not drained: %d", remaining)
		}
		events := inspector.Events()
		if len(events) == 0 || !events[0].BodyLimit || events[0].Blocked != (tc.mode == "block") {
			t.Fatalf("incorrect size event: %+v", events)
		}
	}
}

// Reuse one inspector, as the running service does. Restoring the default
// must also work after a prior transaction has excluded this rule.
func TestSiteRuleExclusionIsTransactionScoped(t *testing.T) {
	inspector := new(Inspector)
	const first = "0000000000000001"
	const second = "0000000000000002"
	for _, tc := range []struct {
		site, mode string
		status     int
	}{
		{first, "block:911100", http.StatusOK},
		{second, "block", http.StatusForbidden},
		{first, "block", http.StatusForbidden},
	} {
		req := httptest.NewRequest("BREW", "http://waf.example.test/", nil)
		req.Header.Set("X-Fnproxy-Instance", "123456abcdef")
		req.Header.Set("X-Fnproxy-Site", tc.site)
		req.Header.Set("X-Fnproxy-Mode", tc.mode)
		req.Header.Set("X-Fnproxy-Target", "generic")
		req.Header.Set("X-Fnproxy-Protocol", "HTTP/1.1")
		req.Header.Set("User-Agent", "nginx-web-test")
		req.Header.Set("Accept", "application/json")
		response := httptest.NewRecorder()
		inspector.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("site %s, mode %s: got %d, want %d", tc.site, tc.mode, response.Code, tc.status)
		}
	}
}
