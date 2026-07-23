package freshdesk

import (
	"net/http"
	"testing"
)

func TestContactList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":1}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "list", "--email", "jane@acme.com", "--company-id", "9", "--updated-since", "2026-02-01T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/contacts" {
		t.Errorf("path = %q, want /contacts", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("email") != "jane@acme.com" || q.Get("company_id") != "9" {
		t.Errorf("filters wrong: %v", got.Query)
	}
	// Contacts use the leading-underscore _updated_since param (distinct from tickets).
	if q.Get("_updated_since") != "2026-02-01T00:00:00Z" {
		t.Errorf("_updated_since = %q, want the timestamp", q.Get("_updated_since"))
	}
}

func TestContactGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":3}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "get", "--id", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/contacts/3" {
		t.Errorf("path = %q, want /contacts/3", got.Path)
	}
}

func TestContactCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":4}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--name", "Jane", "--email", "jane@acme.com", "--company-id", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts" {
		t.Errorf("request = %s %s, want POST /contacts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Jane" || body["email"] != "jane@acme.com" {
		t.Errorf("body = %v", body)
	}
	if body["company_id"] != float64(9) {
		t.Errorf("company_id = %v, want numeric 9", body["company_id"])
	}
}

func TestContactUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":4}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "update", "--id", "4", "--phone", "555-1234")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/contacts/4" {
		t.Errorf("request = %s %s, want PUT /contacts/4", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["phone"] != "555-1234" {
		t.Errorf("phone = %v", body["phone"])
	}
}

func TestContactSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"total":0,"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "search", "--query", "email:'jane@acme.com'")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/search/contacts" {
		t.Errorf("path = %q, want /search/contacts", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("query") != `"email:'jane@acme.com'"` {
		t.Errorf("query = %q, want double-quoted", q.Get("query"))
	}
}
