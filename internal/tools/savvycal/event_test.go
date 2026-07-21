package savvycal

import (
	"net/http"
	"strings"
	"testing"
)

func TestEventList_QueryAndPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[{"id":"evt_1"}],"metadata":{"after":"cur2","before":null,"limit":20}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "event", "list",
		"--state", "all", "--period", "past", "--limit", "50", "--after", "cur1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/events" {
		t.Errorf("request = %s %s, want GET /events", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("state") != "all" || q.Get("period") != "past" || q.Get("limit") != "50" || q.Get("after") != "cur1" {
		t.Errorf("query = %v, want state=all period=past limit=50 after=cur1", q)
	}
	if !strings.Contains(stdout, `"entries"`) || !strings.Contains(stdout, `"metadata"`) {
		t.Errorf("stdout = %q, want full pagination envelope passthrough", stdout)
	}
}

func TestEventList_OmitsUnsetQueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[],"metadata":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	for _, k := range []string{"state", "period", "limit", "after", "before"} {
		if q.Has(k) {
			t.Errorf("query unexpectedly carries %q = %q; unset flags must be omitted", k, q.Get(k))
		}
	}
}

func TestEventGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"evt_9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "get", "evt_9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/events/evt_9" {
		t.Errorf("request = %s %s, want GET /events/evt_9", got.Method, got.Path)
	}
}

func TestEventGet_MissingArg_UsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "get")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for missing positional", code)
	}
}

func TestEventCreate_BodyShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"evt_new"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "create", "link_42",
		"--display-name", "Bob Jones",
		"--email", "bob@acme.co",
		"--start", "2026-08-01T10:00:00Z",
		"--end", "2026-08-01T10:30:00Z",
		"--time-zone", "America/New_York",
		"--field", "q1=blue",
		"--field", "q2=green",
		"--metadata", `{"source":"helio"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/links/link_42/events" {
		t.Errorf("request = %s %s, want POST /links/link_42/events", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["display_name"] != "Bob Jones" || body["email"] != "bob@acme.co" {
		t.Errorf("body identity = %v", body)
	}
	if body["start_at"] != "2026-08-01T10:00:00Z" || body["end_at"] != "2026-08-01T10:30:00Z" {
		t.Errorf("body times = %v", body)
	}
	if body["time_zone"] != "America/New_York" {
		t.Errorf("body time_zone = %v", body["time_zone"])
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 2 {
		t.Fatalf("fields = %v, want 2 entries", body["fields"])
	}
	f0 := fields[0].(map[string]any)
	if f0["id"] != "q1" || f0["value"] != "blue" {
		t.Errorf("fields[0] = %v, want {id:q1,value:blue}", f0)
	}
	meta, ok := body["metadata"].(map[string]any)
	if !ok || meta["source"] != "helio" {
		t.Errorf("metadata = %v, want {source:helio}", body["metadata"])
	}
}

func TestEventCreate_BadFieldFormat_UsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "create", "link_42",
		"--display-name", "Bob",
		"--email", "bob@acme.co",
		"--start", "2026-08-01T10:00:00Z",
		"--end", "2026-08-01T10:30:00Z",
		"--time-zone", "UTC",
		"--field", "novalue")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for malformed --field", code)
	}
}

func TestEventCreate_BadMetadataJSON_UsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "create", "link_42",
		"--display-name", "Bob",
		"--email", "bob@acme.co",
		"--start", "2026-08-01T10:00:00Z",
		"--end", "2026-08-01T10:30:00Z",
		"--time-zone", "UTC",
		"--metadata", "{not json")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for malformed --metadata", code)
	}
}

func TestEventCancel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"evt_9","state":"canceled"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "cancel", "evt_9", "--reason", "conflict")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/events/evt_9/cancel" {
		t.Errorf("request = %s %s, want POST /events/evt_9/cancel", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["cancel_reason"] != "conflict" {
		t.Errorf("body = %v, want cancel_reason=conflict", body)
	}
}

func TestEventCancel_NoReason_EmptyBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"evt_9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "cancel", "evt_9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["cancel_reason"]; ok {
		t.Errorf("body = %v, want no cancel_reason when --reason omitted", body)
	}
}
