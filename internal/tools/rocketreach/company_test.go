package rocketreach

import (
	"net/http"
	"strings"
	"testing"
)

func TestCompanyLookup_ByDomain(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9,"name":"Acme","domain":"acme.com"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "company", "lookup", "--domain", "acme.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v2/company/lookup" {
		t.Errorf("request = %s %s, want GET /api/v2/company/lookup", got.Method, got.Path)
	}
	assertAPIKey(t, got)
	if got.Query.Get("domain") != "acme.com" {
		t.Errorf("query domain = %q, want acme.com", got.Query.Get("domain"))
	}
	if !strings.Contains(stdout, `"domain":"acme.com"`) {
		t.Errorf("stdout = %q, want the provider JSON passthrough", stdout)
	}
}

func TestCompanyLookup_ByName(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9,"name":"Acme"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "lookup", "--name", "Acme")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("name") != "Acme" {
		t.Errorf("query name = %q, want Acme", got.Query.Get("name"))
	}
}

func TestCompanyLookup_NoIdentifier_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "lookup")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing identifier", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestCompanySearch_BuildsQueryBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"companies":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "search", "--name", "Acme", "--page-size", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v2/company/search" {
		t.Errorf("request = %s %s, want POST /api/v2/company/search", got.Method, got.Path)
	}
	assertAPIKey(t, got)
	body := bodyMap(t, got.Body)
	if body["page_size"].(float64) != 5 {
		t.Errorf("page_size = %v, want 5", body["page_size"])
	}
	query := body["query"].(map[string]any)
	name, _ := query["name"].([]any)
	if len(name) != 1 || name[0] != "Acme" {
		t.Errorf("query.name = %v, want [\"Acme\"]", query["name"])
	}
}

func TestCompanySearch_JSONQueryEscapeHatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"companies":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "search", "--json-query", `{"industry":["software"]}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	query := body["query"].(map[string]any)
	ind, _ := query["industry"].([]any)
	if len(ind) != 1 || ind[0] != "software" {
		t.Errorf("query.industry = %v, want [\"software\"] from --json-query", query["industry"])
	}
}

func TestCompanySearch_NoFilters_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "search")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when no filter is given", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}
