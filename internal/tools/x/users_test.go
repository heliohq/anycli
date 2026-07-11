package x

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMe(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":{"id":"2244994945","username":"XDevelopers"}}`)
	})
	defer server.Close()

	code, stdout, _ := run(t, server, fullEnv(), "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/2/users/me" {
		t.Fatalf("request = %s %s, want GET /2/users/me", got.Method, got.Path)
	}
	if got.Auth != "Bearer x-user-token" {
		t.Fatalf("Authorization = %q", got.Auth)
	}
	if stdout != `{"data":{"id":"2244994945","username":"XDevelopers"}}`+"\n" {
		t.Fatalf("stdout was not provider JSON verbatim: %q", stdout)
	}
}

func TestUserGetByIDOrUsername(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "id", args: []string{"user", "get", "--id", "42"}, path: "/2/users/42"},
		{name: "username escaped", args: []string{"user", "get", "--username", "alice"}, path: "/2/users/by/username/alice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				jsonResponse(w, http.StatusOK, `{"data":{"id":"42"}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestUserGetRequiresExactlyOneSelector(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	for _, args := range [][]string{
		{"user", "get"},
		{"user", "get", "--id", "42", "--username", "alice"},
	} {
		code, _, stderr := run(t, server, fullEnv(), args...)
		if code == 0 || !strings.Contains(stderr, "exactly one of --id or --username") {
			t.Fatalf("args %v: code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestUserSearchUsesOneExplicitPage(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		want := url.Values{"query": {"helio"}, "max_results": {"25"}, "next_token": {"page-2"}}
		if query.Encode() != want.Encode() {
			t.Fatalf("query = %q, want %q", query.Encode(), want.Encode())
		}
		jsonResponse(w, http.StatusOK, `{"data":[],"meta":{"next_token":"page-3"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "user", "search", "--query", "helio", "--limit", "25", "--next-token", "page-2")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}
