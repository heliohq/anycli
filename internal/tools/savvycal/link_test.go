package savvycal

import (
	"net/http"
	"strings"
	"testing"
)

func TestLinkList_Pagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[{"id":"link_1"}],"metadata":{"after":"c2"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "link", "list", "--limit", "10", "--before", "c1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/links" {
		t.Errorf("request = %s %s, want GET /links", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "10" || q.Get("before") != "c1" {
		t.Errorf("query = %v, want limit=10 before=c1", q)
	}
	if !strings.Contains(stdout, `"entries"`) {
		t.Errorf("stdout = %q, want envelope passthrough", stdout)
	}
}

func TestLinkGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_7"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "get", "link_7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/links/link_7" {
		t.Errorf("request = %s %s, want GET /links/link_7", got.Method, got.Path)
	}
}

func TestLinkCreate_Personal(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_new"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "create",
		"--name", "Intro Call",
		"--private-name", "Intro (internal)",
		"--description", "30 min chat",
		"--type", "single")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/links" {
		t.Errorf("request = %s %s, want POST /links", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Intro Call" || body["private_name"] != "Intro (internal)" {
		t.Errorf("body names = %v", body)
	}
	if body["description"] != "30 min chat" || body["type"] != "single" {
		t.Errorf("body = %v, want description + type", body)
	}
}

func TestLinkCreate_ScopeRoute(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_new"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "create", "--name", "Team Sync", "--scope", "acme-inc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/scopes/acme-inc/links" {
		t.Errorf("request = %s %s, want POST /scopes/acme-inc/links", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["type"]; ok {
		t.Errorf("body = %v, want no type when --type omitted", body)
	}
}

func TestLinkUpdate_PartialBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_7"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "update", "link_7", "--description", "new desc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/links/link_7" {
		t.Errorf("request = %s %s, want PATCH /links/link_7", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["description"] != "new desc" {
		t.Errorf("body = %v, want description only", body)
	}
	if _, ok := body["name"]; ok {
		t.Errorf("body = %v, want only changed fields present", body)
	}
}

func TestLinkToggle(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_7","state":"disabled"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "toggle", "link_7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/links/link_7/toggle" {
		t.Errorf("request = %s %s, want POST /links/link_7/toggle", got.Method, got.Path)
	}
}

func TestLinkDuplicate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"link_copy"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "duplicate", "link_7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/links/link_7/duplicate" {
		t.Errorf("request = %s %s, want POST /links/link_7/duplicate", got.Method, got.Path)
	}
}

func TestLinkDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "delete", "link_7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/links/link_7" {
		t.Errorf("request = %s %s, want DELETE /links/link_7", got.Method, got.Path)
	}
}

func TestLinkSlots(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`[{"start_at":"2026-08-01T10:00:00Z","end_at":"2026-08-01T10:30:00Z","duration":30,"rank":1}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "link", "slots", "link_7",
		"--from", "2026-08-01T00:00:00Z", "--until", "2026-08-08T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/links/link_7/slots" {
		t.Errorf("request = %s %s, want GET /links/link_7/slots", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("from") != "2026-08-01T00:00:00Z" || q.Get("until") != "2026-08-08T00:00:00Z" {
		t.Errorf("query = %v, want from/until", q)
	}
	if !strings.Contains(stdout, `"rank":1`) {
		t.Errorf("stdout = %q, want slots passthrough", stdout)
	}
}
