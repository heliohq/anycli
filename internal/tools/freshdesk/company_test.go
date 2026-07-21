package freshdesk

import (
	"net/http"
	"testing"
)

func TestCompanyList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":1}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "list", "--per-page", "100")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/companies" {
		t.Errorf("path = %q, want /companies", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("per_page") != "100" {
		t.Errorf("per_page = %q, want 100", q.Get("per_page"))
	}
}

func TestCompanyGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":2}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "get", "--id", "2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/companies/2" {
		t.Errorf("path = %q, want /companies/2", got.Path)
	}
}

func TestCompanySearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"total":0,"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "search", "--query", "name:'Acme'")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/search/companies" {
		t.Errorf("path = %q, want /search/companies", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("query") != `"name:'Acme'"` {
		t.Errorf("query = %q, want double-quoted", q.Get("query"))
	}
}
