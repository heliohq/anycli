package sendgrid

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

func TestTemplateList_DynamicGeneration(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"result":[]}`}, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/templates" {
		t.Errorf("path = %q, want /templates", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("generations") != "dynamic" {
		t.Errorf("generations = %q, want dynamic", q.Get("generations"))
	}
}

func TestTemplateGet_PathEscape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"id":"d-1"}`}, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "get", "--id", "d-1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/templates/d-1" {
		t.Errorf("path = %q, want /templates/d-1", got.Path)
	}
}

func TestContactUpsert_QueuesJobID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusAccepted, body: `{"job_id":"job-9"}`}, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "contact", "upsert",
		"--email", "u@example.com", "--first-name", "Ada",
		"--custom-field", "tier=gold")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/marketing/contacts" {
		t.Errorf("request = %s %s, want PUT /marketing/contacts", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"job_id":"job-9"`) {
		t.Errorf("stdout = %q, want the job_id passthrough", stdout)
	}
	body := decodeBody(t, got.Body)
	contacts := body["contacts"].([]any)
	c := contacts[0].(map[string]any)
	if c["email"] != "u@example.com" || c["first_name"] != "Ada" {
		t.Errorf("contact = %v, want email+first_name", c)
	}
	fields := c["custom_fields"].(map[string]any)
	if fields["tier"] != "gold" {
		t.Errorf("custom_fields = %v, want tier=gold", fields)
	}
}

func TestContactSearch_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"result":{}}`}, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "search", "--email", "a@b.com", "--email", "c@d.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/marketing/contacts/search/emails" {
		t.Errorf("request = %s %s, want POST /marketing/contacts/search/emails", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	emails := body["emails"].([]any)
	if len(emails) != 2 || emails[0] != "a@b.com" {
		t.Errorf("emails = %v, want [a@b.com c@d.com]", emails)
	}
}

func TestSuppression_Routes(t *testing.T) {
	cases := []struct {
		sub  string
		path string
	}{
		{"bounces", "/suppression/bounces"},
		{"unsubscribes", "/suppression/unsubscribes"},
		{"blocks", "/suppression/blocks"},
	}
	for _, tc := range cases {
		t.Run(tc.sub, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, serverResponse{status: http.StatusOK, body: `[]`}, &got)
			defer srv.Close()

			code, _, _ := run(t, srv, "suppression", tc.sub, "--limit", "5", "--offset", "10")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if got.Method != http.MethodGet || got.Path != tc.path {
				t.Errorf("request = %s %s, want GET %s", got.Method, got.Path, tc.path)
			}
			q := parseQuery(t, got.Query)
			if q.Get("limit") != "5" || q.Get("offset") != "10" {
				t.Errorf("query = %q, want limit=5 offset=10", got.Query)
			}
		})
	}
}

func TestStats_RequiresStartDate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `[]`}, &got)
	defer srv.Close()

	// Missing --start-date is a cobra usage error; Service.Execute maps every
	// command error through Failure → non-zero exit (the engine surfaces the
	// usage class downstream). Assert it does not succeed and never hits the API.
	code, _, stderr := run(t, srv, "stats")
	if code == 0 {
		t.Fatalf("exit code = %d, want non-zero for missing required flag", code)
	}
	if !strings.Contains(stderr, "start-date") {
		t.Errorf("stderr = %q, want a required-flag error", stderr)
	}
	if got.Path != "" {
		t.Errorf("missing required flag must not reach the API; path=%q", got.Path)
	}

	code, _, _ = run(t, srv, "stats", "--start-date", "2026-01-01", "--aggregated-by", "week")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("start_date") != "2026-01-01" || q.Get("aggregated_by") != "week" {
		t.Errorf("query = %q, want start_date + aggregated_by", got.Query)
	}
}

func TestSenderList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"results":[]}`}, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "sender", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/verified_senders" {
		t.Errorf("path = %q, want /verified_senders", got.Path)
	}
}

func TestListLs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"result":[]}`}, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/marketing/lists" {
		t.Errorf("path = %q, want /marketing/lists", got.Path)
	}
}
