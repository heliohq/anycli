package intercom

import (
	"net/http"
	"testing"
)

func TestContactCreate_ScalarFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"contact","id":"c1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--email", "a@b.com", "--name", "Ada", "--role", "lead")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@b.com" || body["name"] != "Ada" || body["role"] != "lead" {
		t.Errorf("body = %v", body)
	}
}

func TestContactCreate_BodyJSONMergeWins(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"contact"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--email", "a@b.com",
		"--body-json", `{"email":"override@b.com","custom_attributes":{"plan":"pro"}}`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "override@b.com" {
		t.Errorf("email = %v, want body-json override", body["email"])
	}
	if _, ok := body["custom_attributes"].(map[string]any); !ok {
		t.Errorf("custom_attributes = %v, want merged object", body["custom_attributes"])
	}
}

func TestContactUpdate_PUTDropsUnsetRoleDefault(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"contact"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "update", "--id", "c1", "--name", "New")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/contacts/c1" {
		t.Errorf("request = %s %s, want PUT /contacts/c1", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["role"]; ok {
		t.Errorf("role should be dropped on update when not explicitly set, body = %v", body)
	}
	if body["name"] != "New" {
		t.Errorf("name = %v", body["name"])
	}
}

func TestContactSearch_EmailFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"list"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "search", "--email", "a@b.com")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/contacts/search" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	q := body["query"].(map[string]any)
	if q["field"] != "email" || q["value"] != "a@b.com" {
		t.Errorf("query = %v, want single email equality filter", q)
	}
}

func TestContactNote(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"note"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "note", "--id", "c1", "--body", "vip", "--admin-id", "7")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts/c1/notes" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["body"] != "vip" || body["admin_id"] != "7" {
		t.Errorf("body = %v", body)
	}
}
