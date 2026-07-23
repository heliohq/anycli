package hunter

import (
	"net/http"
	"testing"
)

func TestEmailFinder_NameAndDomain(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"email":"jane@stripe.com"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "email-finder",
		"--domain", "stripe.com", "--first-name", "Jane", "--last-name", "Doe", "--max-duration", "15")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/email-finder" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("domain") != "stripe.com" || q.Get("first_name") != "Jane" || q.Get("last_name") != "Doe" {
		t.Errorf("query = %s (kebab flags must map to snake_case)", got.Query)
	}
	if q.Get("max_duration") != "15" {
		t.Errorf("max_duration = %q", q.Get("max_duration"))
	}
	if got.APIKey != "key-123" {
		t.Errorf("X-API-KEY = %q", got.APIKey)
	}
	if stdout == "" {
		t.Error("want passthrough")
	}
}

func TestEmailFinder_LinkedinHandle(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "email-finder", "--linkedin-handle", "janedoe")
	q := parseQuery(t, got.Query)
	if q.Get("linkedin_handle") != "janedoe" {
		t.Errorf("linkedin_handle = %q", q.Get("linkedin_handle"))
	}
	if q.Has("max_duration") {
		t.Errorf("unset max-duration should be omitted; query = %s", got.Query)
	}
}
