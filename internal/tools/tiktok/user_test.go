package tiktok

import (
	"net/http"
	"strings"
	"testing"
)

func TestUserInfoRequestAndUnwrap(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"user":{"open_id":"o123","display_name":"Creator"}}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "user", "info")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/user/info/" {
		t.Fatalf("request = %s %s, want GET /v2/user/info/", got.Method, got.Path)
	}
	if got.Auth != "Bearer act.user-token" {
		t.Fatalf("auth header = %q", got.Auth)
	}
	if !strings.Contains(got.RawQuery, "fields=") || !strings.Contains(got.RawQuery, "open_id") {
		t.Fatalf("query = %q, want a fields param including open_id", got.RawQuery)
	}
	// The wrapped {data:{user:{…}}} envelope is unwrapped to the user object.
	if !strings.Contains(stdout, `"open_id":"o123"`) || strings.Contains(stdout, `"user"`) {
		t.Fatalf("stdout = %q, want the flat user object", stdout)
	}
}

func TestUserInfoCustomFields(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"user":{"open_id":"o1"}}`))
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "user", "info", "--fields", "open_id,follower_count")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(got.RawQuery, "follower_count") {
		t.Fatalf("query = %q, want custom fields", got.RawQuery)
	}
}
