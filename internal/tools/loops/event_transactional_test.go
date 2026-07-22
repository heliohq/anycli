package loops

import (
	"net/http"
	"testing"
)

func TestEventSend_PropertiesMailingListIdempotency(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "send",
		"--event-name", "signup", "--email", "a@e.com",
		"--event-property", "plan=Pro", "--event-property", "count=3",
		"--event-properties-json", `{"src":"web"}`,
		"--mailing-list", "l1=true",
		"--idempotency-key", "idem-1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/events/send" {
		t.Errorf("request = %s %s, want POST /v1/events/send", got.Method, got.Path)
	}
	if got.Idempotency != "idem-1" {
		t.Errorf("Idempotency-Key = %q, want idem-1", got.Idempotency)
	}
	body := decodeBody(t, got.Body)
	if body["eventName"] != "signup" || body["email"] != "a@e.com" {
		t.Errorf("body = %v", body)
	}
	props, ok := body["eventProperties"].(map[string]any)
	if !ok || props["plan"] != "Pro" || props["count"] != float64(3) || props["src"] != "web" {
		t.Errorf("eventProperties = %v", body["eventProperties"])
	}
	mailing, ok := body["mailingLists"].(map[string]any)
	if !ok || mailing["l1"] != true {
		t.Errorf("mailingLists = %v", body["mailingLists"])
	}
}

func TestEventSend_RequiresEventNameAndIdentifier(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// missing identifier
	code, _, _ := run(t, srv, "event", "send", "--event-name", "signup")
	if code != 2 {
		t.Fatalf("missing identifier: exit code = %d, want 2", code)
	}
	// missing event-name
	got = capturedRequest{}
	code, _, _ = run(t, srv, "event", "send", "--email", "a@e.com")
	if code != 2 {
		t.Fatalf("missing event-name: exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
}

func TestEmailSend_DataVariablesAndAudience(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--email", "a@e.com", "--transactional-id", "tmpl_1",
		"--data-variable", "name=Chris", "--data-variable", "count=2",
		"--data-variables-json", `{"link":"https://x"}`,
		"--add-to-audience",
		"--idempotency-key", "idem-2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/transactional" {
		t.Errorf("request = %s %s, want POST /v1/transactional", got.Method, got.Path)
	}
	if got.Idempotency != "idem-2" {
		t.Errorf("Idempotency-Key = %q, want idem-2", got.Idempotency)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@e.com" || body["transactionalId"] != "tmpl_1" {
		t.Errorf("body = %v", body)
	}
	if body["addToAudience"] != true {
		t.Errorf("addToAudience = %v, want true", body["addToAudience"])
	}
	vars, ok := body["dataVariables"].(map[string]any)
	if !ok || vars["name"] != "Chris" || vars["count"] != float64(2) || vars["link"] != "https://x" {
		t.Errorf("dataVariables = %v", body["dataVariables"])
	}
}

func TestEmailSend_RequiresEmailAndTemplate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send", "--email", "a@e.com")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (missing --transactional-id)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
}

func TestEmailList_Query(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "list", "--per-page", "30", "--cursor", "abc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/transactional" {
		t.Errorf("request = %s %s, want GET /v1/transactional", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("perPage") != "30" || q.Get("cursor") != "abc" {
		t.Errorf("query = %q, want perPage=30 cursor=abc", got.Query)
	}
}

// TestBadKeyValue is a client-side usage error for a malformed key=value pair.
func TestBadKeyValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "send", "--event-name", "e", "--email", "a@e.com",
		"--event-property", "novalue")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (malformed key=value)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
}
