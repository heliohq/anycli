package hunter

import (
	"net/http"
	"testing"
)

func TestDomainSearch_QueryShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"emails":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain-search",
		"--domain", "stripe.com", "--department", "engineering,it",
		"--seniority", "senior", "--type", "personal", "--limit", "10", "--offset", "5")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/domain-search" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("domain") != "stripe.com" || q.Get("department") != "engineering,it" {
		t.Errorf("query = %s", got.Query)
	}
	if q.Get("seniority") != "senior" || q.Get("type") != "personal" {
		t.Errorf("query = %s", got.Query)
	}
	if q.Get("limit") != "10" || q.Get("offset") != "5" {
		t.Errorf("limit/offset = %s", got.Query)
	}
	if q.Has("api_key") {
		t.Error("api_key must never appear in the query")
	}
}

func TestDomainSearch_OmitsUnsetLimitOffset(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "domain-search", "--company", "Stripe")
	q := parseQuery(t, got.Query)
	if q.Has("limit") || q.Has("offset") {
		t.Errorf("unset limit/offset should be omitted; query = %s", got.Query)
	}
	if q.Get("company") != "Stripe" {
		t.Errorf("company = %q", q.Get("company"))
	}
}

func TestEmailCount_Free(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"total":42}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "email-count", "--domain", "stripe.com", "--type", "generic")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/email-count" {
		t.Errorf("path = %s", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("domain") != "stripe.com" || q.Get("type") != "generic" {
		t.Errorf("query = %s", got.Query)
	}
	if stdout == "" {
		t.Error("want passthrough body")
	}
}

func TestDomainFinder_RequiredCompanyAndPerfectMatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"domain":"stripe.com"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain-finder", "--company", "Stripe", "--limit", "3", "--perfect-match")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/domain-finder" {
		t.Errorf("path = %s", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("company") != "Stripe" || q.Get("limit") != "3" || q.Get("perfect_match") != "true" {
		t.Errorf("query = %s", got.Query)
	}
}

func TestDomainFinder_MissingCompanyIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "domain-finder")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Error("should not call API without required --company")
	}
}
