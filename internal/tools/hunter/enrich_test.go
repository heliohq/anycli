package hunter

import (
	"net/http"
	"testing"
)

func TestEnrichPerson_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"p1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "enrich", "person", "--email", "jane@stripe.com")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/people/find" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("email") != "jane@stripe.com" {
		t.Errorf("email = %q", q.Get("email"))
	}
}

func TestEnrichCompany_ByDomain(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"domain":"stripe.com"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "enrich", "company", "--domain", "stripe.com")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/companies/find" {
		t.Errorf("path = %s", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("domain") != "stripe.com" {
		t.Errorf("domain = %q", q.Get("domain"))
	}
}

func TestEnrichCombined_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "enrich", "combined", "--email", "jane@stripe.com")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/combined/find" {
		t.Errorf("path = %s", got.Path)
	}
}

func TestEnrich_404IsPlainRuntimeError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound,
		`{"errors":[{"id":"not_found","details":"No results."}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "enrich", "person", "--email", "nobody@nowhere.com")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("404 must not reject the credential")
	}
	if stderr == "" {
		t.Error("want error output")
	}
}
