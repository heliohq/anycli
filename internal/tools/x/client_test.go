package x

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIErrorsIncludeStatusBodyAndSafeHints(t *testing.T) {
	tests := []struct {
		status int
		body   string
		hint   string
	}{
		{status: http.StatusUnauthorized, body: `{"title":"Unauthorized","detail":"token expired: x-user-token"}`, hint: "access token is invalid or expired"},
		{status: http.StatusForbidden, body: `{"title":"Forbidden","detail":"client-not-enrolled"}`, hint: "required scope or account permission"},
		{status: http.StatusTooManyRequests, body: `{"title":"Too Many Requests","detail":"rate cap"}`, hint: "rate limit exceeded"},
	}
	for _, tt := range tests {
		server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, tt.status, tt.body)
		})
		code, _, stderr := run(t, server, fullEnv(), "me")
		server.Close()
		if code == 0 {
			t.Fatalf("status %d returned exit 0", tt.status)
		}
		if !strings.Contains(stderr, "HTTP ") || !strings.Contains(stderr, tt.hint) {
			t.Fatalf("status %d stderr = %q", tt.status, stderr)
		}
		if strings.Contains(stderr, "x-user-token") {
			t.Fatalf("status %d leaked bearer token: %q", tt.status, stderr)
		}
		if !strings.Contains(stderr, `"title"`) {
			t.Fatalf("status %d lost provider error body: %q", tt.status, stderr)
		}
	}
}
