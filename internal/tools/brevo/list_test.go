package brevo

import (
	"net/http"
	"testing"
)

func TestListLs_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"lists":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "ls", "--limit", "25", "--offset", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/contacts/lists" {
		t.Errorf("request = %s %s, want GET /contacts/lists", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "25" || q.Get("offset") != "5" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestListGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9,"name":"Newsletter"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "get", "--id", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/contacts/lists/9" {
		t.Errorf("request = %s %s, want GET /contacts/lists/9", got.Method, got.Path)
	}
}

func TestListCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":10}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "create", "--name", "Newsletter", "--folder-id", "2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts/lists" {
		t.Errorf("request = %s %s, want POST /contacts/lists", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Newsletter" || body["folderId"] != float64(2) {
		t.Errorf("body = %v, want {name,folderId}", body)
	}
}

func TestListAddContacts_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"contacts":{"success":["a@b.com"]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "add-contacts", "--id", "9",
		"--emails", "a@b.com", "--emails", "c@d.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts/lists/9/contacts/add" {
		t.Errorf("request = %s %s, want POST /contacts/lists/9/contacts/add", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	emails, ok := body["emails"].([]any)
	if !ok || len(emails) != 2 || emails[0] != "a@b.com" {
		t.Errorf("emails = %v", body["emails"])
	}
}

func TestListAddContacts_ByID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "add-contacts", "--id", "9", "--ids", "100", "--ids", "200")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	ids, ok := body["ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != float64(100) {
		t.Errorf("ids = %v, want [100,200] as integers", body["ids"])
	}
	if _, ok := body["emails"]; ok {
		t.Errorf("emails should be omitted when only --ids given, body = %v", body)
	}
}
