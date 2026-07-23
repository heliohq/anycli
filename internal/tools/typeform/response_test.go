package typeform

import (
	"net/http"
	"strings"
	"testing"
)

func TestResponseListPassesFiltersAndCursors(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1/responses": {status: 200, body: `{"total_items":0,"page_count":0,"items":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "response", "list", "f1",
		"--page-size", "100", "--since", "2020-01-01T00:00:00", "--until", "2020-02-01T00:00:00",
		"--after", "curA", "--query", "hello",
		"--response-type", "partial,completed",
		"--fields", "a,b", "--answered-fields", "c",
		"--included-response-ids", "r1,r2", "--excluded-response-ids", "r3")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/forms/f1/responses")
	if req == nil {
		t.Fatal("no GET /forms/f1/responses recorded")
	}
	q := req.Query
	checks := map[string]string{
		"page_size": "100", "since": "2020-01-01T00:00:00", "until": "2020-02-01T00:00:00",
		"after": "curA", "query": "hello", "response_type": "partial,completed",
		"fields": "a,b", "answered_fields": "c",
		"included_response_ids": "r1,r2", "excluded_response_ids": "r3",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestResponseListSortSpecPassedThrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1/responses": {status: 200, body: `{"items":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "response", "list", "f1", "--sort", "submitted_at,desc")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/forms/f1/responses")
	if req == nil || req.Query.Get("sort") != "submitted_at,desc" {
		t.Fatalf("sort not passed through: %+v", req)
	}
}

func TestResponseListRejectsBadResponseType(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "response", "list", "f1", "--response-type", "completed,bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for bad response-type member", exit)
	}
	if !strings.Contains(stderr, "response-type") {
		t.Errorf("stderr = %q, want enum message", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("made %d requests on parse error, want 0", len(reqs))
	}
}
