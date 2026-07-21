package brevo

import (
	"net/http"
	"testing"
)

func TestContactCreate_UpsertBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create",
		"--email", "jane@acme.com", "--update-enabled",
		"--list-ids", "3", "--list-ids", "5",
		"--attributes-json", `{"FIRSTNAME":"Jane"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts" {
		t.Errorf("request = %s %s, want POST /contacts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "jane@acme.com" {
		t.Errorf("email = %v", body["email"])
	}
	if body["updateEnabled"] != true {
		t.Errorf("updateEnabled = %v, want true", body["updateEnabled"])
	}
	listIDs, ok := body["listIds"].([]any)
	if !ok || len(listIDs) != 2 || listIDs[0] != float64(3) {
		t.Errorf("listIds = %v, want [3,5] as integers", body["listIds"])
	}
	attrs, ok := body["attributes"].(map[string]any)
	if !ok || attrs["FIRSTNAME"] != "Jane" {
		t.Errorf("attributes = %v", body["attributes"])
	}
}

func TestContactCreate_UpdateDisabledOmittedWhenNotSet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":1}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--email", "a@b.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["updateEnabled"]; ok {
		t.Errorf("updateEnabled should be omitted when flag unset, body = %v", body)
	}
	if _, ok := body["listIds"]; ok {
		t.Errorf("listIds should be omitted when empty, body = %v", body)
	}
}

func TestContactGet_IdentifierPathAndType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"email":"jane@acme.com"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "get", "--id", "42", "--identifier-type", "contact_id")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/contacts/42" {
		t.Errorf("request = %s %s, want GET /contacts/42", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("identifierType") != "contact_id" {
		t.Errorf("identifierType = %q, want contact_id", q.Get("identifierType"))
	}
}

func TestContactGet_EmailIdentifierEscaped(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "get", "--id", "jane+tag@acme.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/contacts/jane+tag@acme.com" {
		t.Errorf("path = %q, want /contacts/jane+tag@acme.com (escaped)", got.Path)
	}
}

func TestContactUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "update", "--id", "jane@acme.com",
		"--attributes-json", `{"SMS":"+123"}`, "--list-ids", "7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/contacts/jane@acme.com" {
		t.Errorf("request = %s %s, want PUT /contacts/jane@acme.com", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if listIDs, ok := body["listIds"].([]any); !ok || listIDs[0] != float64(7) {
		t.Errorf("listIds = %v", body["listIds"])
	}
}

func TestContactList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"contacts":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "list", "--limit", "10", "--offset", "20", "--sort", "desc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/contacts" {
		t.Errorf("path = %q, want /contacts", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "10" || q.Get("offset") != "20" || q.Get("sort") != "desc" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestContactDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "delete", "--id", "42", "--identifier-type", "contact_id")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/contacts/42" {
		t.Errorf("request = %s %s, want DELETE /contacts/42", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("identifierType") != "contact_id" {
		t.Errorf("identifierType = %q", q.Get("identifierType"))
	}
}
