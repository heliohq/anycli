package freshdesk

import (
	"net/http"
	"strings"
	"testing"
)

func TestAgentList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":1}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "agent", "list", "--email", "agent@acme.com", "--page", "1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/agents" {
		t.Errorf("path = %q, want /agents", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("email") != "agent@acme.com" {
		t.Errorf("email = %q", q.Get("email"))
	}
}

func TestAgentGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":2}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "agent", "get", "--id", "2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/agents/2" {
		t.Errorf("path = %q, want /agents/2", got.Path)
	}
}

func TestAgentMe(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":3,"contact":{"name":"Me","email":"me@acme.com"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "agent", "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/agents/me" {
		t.Errorf("request = %s %s, want GET /agents/me", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"name":"Me"`) {
		t.Errorf("stdout = %q, want JSON passthrough", stdout)
	}
}
