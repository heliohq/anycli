package calendly

import (
	"net/http"
	"strings"
	"testing"
)

func TestEventListFiltersAndPaging(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"collection":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv,
		"event", "list",
		"--user", "U-1",
		"--status", "active",
		"--invitee-email", "a@b.com",
		"--from", "2026-08-01T00:00:00Z",
		"--to", "2026-08-31T00:00:00Z",
		"--page-token", "PT",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Path != "/scheduled_events" {
		t.Fatalf("path = %q, want /scheduled_events", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("user") != srv.URL+"/users/U-1" {
		t.Errorf("user = %q", q.Get("user"))
	}
	if q.Get("status") != "active" || q.Get("invitee_email") != "a@b.com" {
		t.Errorf("status/invitee_email wrong: %q", got.Query)
	}
	if q.Get("min_start_time") != "2026-08-01T00:00:00Z" || q.Get("max_start_time") != "2026-08-31T00:00:00Z" {
		t.Errorf("min/max_start_time wrong: %q", got.Query)
	}
	if q.Get("page_token") != "PT" {
		t.Errorf("page_token = %q, want PT", q.Get("page_token"))
	}
}

func TestEventGetPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"resource":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "get", "EV-3")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/scheduled_events/EV-3" {
		t.Errorf("path = %q, want /scheduled_events/EV-3", got.Path)
	}
}

func TestEventInviteesPathAndFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"collection":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "invitees", "EV-3", "--status", "active", "--count", "5")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/scheduled_events/EV-3/invitees" {
		t.Errorf("path = %q, want /scheduled_events/EV-3/invitees", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("status") != "active" || q.Get("count") != "5" {
		t.Errorf("query = %q, want status=active count=5", got.Query)
	}
}

func TestEventCancelSendsReasonBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"resource":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "cancel", "EV-3", "--reason", "conflict")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/scheduled_events/EV-3/cancellation" {
		t.Errorf("request = %s %s, want POST …/cancellation", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["reason"] != "conflict" {
		t.Errorf("reason = %v, want conflict", body["reason"])
	}
}

func TestEventCancelOmitsEmptyReason(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"resource":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "cancel", "EV-3")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if strings.Contains(string(got.Body), "reason") {
		t.Errorf("body = %s, want no reason field when empty", got.Body)
	}
}
