package formstack

import (
	"net/http"
	"strings"
	"testing"
)

func TestFormList_QueryAndAuth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"forms":[],"total":0}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "form", "list", "--search", "intake", "--folder", "42", "--page", "2", "--per-page", "50", "--sort", "name-asc")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/form.json" {
		t.Errorf("request = %s %s, want GET /form.json", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("auth = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("accept = %q", got.Accept)
	}
	q := parseQuery(t, got.Query)
	if q.Get("search") != "intake" || q.Get("folder") != "42" || q.Get("page") != "2" || q.Get("per_page") != "50" || q.Get("sort") != "name-asc" {
		t.Errorf("query = %q", got.Query)
	}
	if !strings.Contains(stdout, `"total":0`) {
		t.Errorf("stdout passthrough = %q", stdout)
	}
}

func TestFormList_OmitsUnsetPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"forms":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	q := parseQuery(t, got.Query)
	if _, ok := q["page"]; ok {
		t.Errorf("page should be absent when unset, query = %q", got.Query)
	}
	if _, ok := q["per_page"]; ok {
		t.Errorf("per_page should be absent when unset, query = %q", got.Query)
	}
}

func TestFormGet_JSONSuffixPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"12345"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "get", "12345"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/form/12345.json" {
		t.Errorf("request = %s %s, want GET /form/12345.json", got.Method, got.Path)
	}
}

func TestFormFields_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "fields", "12345"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/form/12345/field.json" {
		t.Errorf("path = %q, want /form/12345/field.json", got.Path)
	}
}

func TestFormCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"99"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "create", "--name", "RSVP", "--folder", "7"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/form.json" {
		t.Errorf("request = %s %s, want POST /form.json", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "RSVP" || body["folder"] != "7" {
		t.Errorf("body = %v", body)
	}
}

func TestFormCopy_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"100"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "copy", "99"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/form/99/copy.json" {
		t.Errorf("request = %s %s, want POST /form/99/copy.json", got.Method, got.Path)
	}
}

func TestFormDelete_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":"1"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "delete", "99"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/form/99.json" {
		t.Errorf("request = %s %s, want DELETE /form/99.json", got.Method, got.Path)
	}
}
