package freshservice

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTicketListPaginationAndNextPage(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {
			body: `{"tickets":[{"id":1},{"id":2}]}`,
			// A next-page link header must surface as next_page = page+1.
			headers: map[string]string{"Link": `<https://acme.freshservice.com/api/v2/tickets?page=3>; rel="next"`},
		},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "ticket", "list", "--per-page", "2", "--page", "2", "--updated-since", "2026-01-01T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	q, _ := url.ParseQuery(captured["/tickets"].Query)
	if q.Get("per_page") != "2" || q.Get("page") != "2" {
		t.Fatalf("query = %q, want per_page=2 page=2", captured["/tickets"].Query)
	}
	if q.Get("updated_since") != "2026-01-01T00:00:00Z" {
		t.Fatalf("updated_since not sent: %q", captured["/tickets"].Query)
	}
	m := decodeJSON(t, out)
	if m["page"].(float64) != 2 || m["per_page"].(float64) != 2 {
		t.Fatalf("output page/per_page wrong: %v", m)
	}
	if m["next_page"].(float64) != 3 {
		t.Fatalf("next_page = %v, want 3", m["next_page"])
	}
	if items, ok := m["items"].([]any); !ok || len(items) != 2 {
		t.Fatalf("items wrong: %v", m["items"])
	}
}

func TestTicketListNoNextPageWhenNoLink(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {body: `{"tickets":[{"id":1}]}`},
	}, captured)
	defer srv.Close()

	code, out, _ := run(t, srv, "ticket", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	m := decodeJSON(t, out)
	if m["next_page"] != nil {
		t.Fatalf("next_page = %v, want null", m["next_page"])
	}
}

func TestTicketListRejectsPerPageOver100(t *testing.T) {
	code, _, errStr := run(t, nil, "ticket", "list", "--per-page", "101")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errStr)
	}
}

func TestTicketSearchSendsQuotedQueryNoPerPage(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/filter": {body: `{"tickets":[{"id":9}],"total":1}`},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "ticket", "search", "--query", "status:2 AND priority:1", "--page", "2")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	q, _ := url.ParseQuery(captured["/tickets/filter"].Query)
	if got := q.Get("query"); got != `"status:2 AND priority:1"` {
		t.Fatalf("query = %q, want quoted expression", got)
	}
	if q.Has("per_page") {
		t.Fatalf("search must not send per_page: %q", captured["/tickets/filter"].Query)
	}
	if q.Get("page") != "2" {
		t.Fatalf("page = %q, want 2", q.Get("page"))
	}
	m := decodeJSON(t, out)
	if m["per_page"].(float64) != 30 {
		t.Fatalf("search per_page should report the fixed 30: %v", m["per_page"])
	}
	// The filter endpoint's body `total` must surface so the agent can page
	// deterministically (there is no Link header on /tickets/filter).
	if m["total"].(float64) != 1 {
		t.Fatalf("search must surface the body total: %v", m["total"])
	}
}

// TestTicketSearchNextPageFromTotal proves search derives next_page from the
// body `total` (GET /tickets/filter sends no Link header): it advances while the
// total still has rows on a later page, and stops both when the total is
// exhausted and at the hard 10-page cap. A Link-header-only projection would
// return next_page=null here and silently cap the agent at the first 30 rows.
func TestTicketSearchNextPageFromTotal(t *testing.T) {
	cases := []struct {
		name     string
		total    int
		page     int
		wantNext any
	}{
		{name: "more results remain", total: 65, page: 1, wantNext: float64(2)},
		{name: "middle page advances", total: 65, page: 2, wantNext: float64(3)},
		{name: "last partial page has no next", total: 65, page: 3, wantNext: nil},
		{name: "exact page boundary has no next", total: 60, page: 2, wantNext: nil},
		{name: "hard cap at page 10", total: 350, page: 10, wantNext: nil},
		{name: "page before cap advances", total: 350, page: 9, wantNext: float64(10)},
		{name: "single result no next", total: 1, page: 1, wantNext: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured := map[string]capturedRequest{}
			srv := newFakeServer(t, map[string]routeReply{
				"/tickets/filter": {body: fmt.Sprintf(`{"tickets":[{"id":9}],"total":%d}`, tc.total)},
			}, captured)
			defer srv.Close()

			code, out, errStr := run(t, srv, "ticket", "search", "--query", "status:2", "--page", strconv.Itoa(tc.page))
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
			}
			m := decodeJSON(t, out)
			if m["total"].(float64) != float64(tc.total) {
				t.Fatalf("total = %v, want %d", m["total"], tc.total)
			}
			if !reflect.DeepEqual(m["next_page"], tc.wantNext) {
				t.Fatalf("next_page = %v (%T), want %v", m["next_page"], m["next_page"], tc.wantNext)
			}
		})
	}
}

func TestTicketSearchRejectsPageOutOfRange(t *testing.T) {
	for _, page := range []string{"0", "11"} {
		code, _, errStr := run(t, nil, "ticket", "search", "--query", "status:2", "--page", page)
		if code != 2 {
			t.Fatalf("page=%s exit = %d, want 2 (stderr: %s)", page, code, errStr)
		}
	}
}

func TestTicketSearchRequiresQuery(t *testing.T) {
	code, _, _ := run(t, nil, "ticket", "search")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestTicketGetWithConversations(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/42": {body: `{"ticket":{"id":42,"subject":"printer down"}}`},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "ticket", "get", "42", "--conversations")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	q, _ := url.ParseQuery(captured["/tickets/42"].Query)
	if q.Get("include") != "conversations" {
		t.Fatalf("include = %q, want conversations", q.Get("include"))
	}
	// Resource envelope must be unwrapped to the bare ticket object.
	m := decodeJSON(t, out)
	if m["id"].(float64) != 42 || m["subject"] != "printer down" {
		t.Fatalf("get output not unwrapped: %v", m)
	}
}

func TestTicketCreateAppliesDefaults(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {status: 201, body: `{"ticket":{"id":100}}`},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "ticket", "create",
		"--subject", "VPN broken", "--description", "cannot connect", "--email", "user@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	body := decodeBody(t, captured["/tickets"].Body)
	if body["status"].(float64) != 2 || body["priority"].(float64) != 2 {
		t.Fatalf("defaults not applied: %v", body)
	}
	if body["subject"] != "VPN broken" || body["email"] != "user@acme.com" {
		t.Fatalf("body fields wrong: %v", body)
	}
	// No responder_id / group_id / type when unset.
	if _, ok := body["responder_id"]; ok {
		t.Fatalf("responder_id should be omitted when unset: %v", body)
	}
	if _, ok := body["type"]; ok {
		t.Fatalf("type should be omitted when unset: %v", body)
	}
	m := decodeJSON(t, out)
	if m["id"].(float64) != 100 {
		t.Fatalf("create output not unwrapped: %v", m)
	}
}

func TestTicketCreateHonoursOverrides(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {status: 201, body: `{"ticket":{"id":101}}`},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "ticket", "create",
		"--subject", "s", "--description", "d", "--email", "e@acme.com",
		"--status", "3", "--priority", "4", "--group-id", "55", "--agent-id", "66", "--type", "Service Request")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	body := decodeBody(t, captured["/tickets"].Body)
	if body["status"].(float64) != 3 || body["priority"].(float64) != 4 {
		t.Fatalf("overrides not applied: %v", body)
	}
	if body["group_id"].(float64) != 55 || body["responder_id"].(float64) != 66 {
		t.Fatalf("group/agent not mapped: %v", body)
	}
	if body["type"] != "Service Request" {
		t.Fatalf("type = %v, want Service Request", body["type"])
	}
}

func TestTicketCreateRequiresCoreFields(t *testing.T) {
	code, _, _ := run(t, nil, "ticket", "create", "--subject", "only subject")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestTicketUpdateSendsOnlyChangedFields(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/7": {body: `{"ticket":{"id":7}}`},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "ticket", "update", "7", "--status", "4", "--tags", "vpn,urgent")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	if captured["/tickets/7"].Method != "PUT" {
		t.Fatalf("method = %q, want PUT", captured["/tickets/7"].Method)
	}
	body := decodeBody(t, captured["/tickets/7"].Body)
	if body["status"].(float64) != 4 {
		t.Fatalf("status not sent: %v", body)
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "vpn" {
		t.Fatalf("tags not sent: %v", body)
	}
	// priority / group_id / responder_id must be absent (not changed).
	if _, ok := body["priority"]; ok {
		t.Fatalf("priority should be absent: %v", body)
	}
}

func TestTicketUpdateNeedsAtLeastOneField(t *testing.T) {
	code, _, errStr := run(t, nil, "ticket", "update", "7")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errStr)
	}
}

func TestTicketReply(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/5/reply": {status: 201, body: `{"conversation":{"id":900}}`},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "ticket", "reply", "5", "--body", "we are on it")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	body := decodeBody(t, captured["/tickets/5/reply"].Body)
	if body["body"] != "we are on it" {
		t.Fatalf("reply body wrong: %v", body)
	}
	m := decodeJSON(t, out)
	if m["id"].(float64) != 900 {
		t.Fatalf("reply output not unwrapped: %v", m)
	}
}

func TestTicketNoteDefaultsPrivate(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/5/notes": {status: 201, body: `{"conversation":{"id":901}}`},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "ticket", "note", "5", "--body", "internal only")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	body := decodeBody(t, captured["/tickets/5/notes"].Body)
	if body["private"] != true {
		t.Fatalf("note should default private=true: %v", body)
	}
}

func TestTicketNotePublicOverride(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets/5/notes": {status: 201, body: `{"conversation":{"id":902}}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "note", "5", "--body", "public", "--private=false")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	body := decodeBody(t, captured["/tickets/5/notes"].Body)
	if body["private"] != false {
		t.Fatalf("note private override failed: %v", body)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {status: 401, body: `{"code":"invalid_credentials","message":"Please provide a valid API key"}`},
	}, captured)
	defer srv.Close()

	res, _, errStr := runResult(t, srv, "ticket", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", res.ExitCode, errStr)
	}
	if !res.CredentialRejected {
		t.Fatalf("401 should mark the credential rejected")
	}
}

func TestRateLimitSurfacesRetryAfter(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {
			status:  429,
			body:    `{"description":"Rate limit exceeded"}`,
			headers: map[string]string{"Retry-After": "42"},
		},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "--json", "ticket", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, errStr)
	}
	m := decodeJSON(t, strings.TrimSpace(errStr))
	errObj := m["error"].(map[string]any)
	if errObj["status"].(float64) != 429 {
		t.Fatalf("status = %v, want 429", errObj["status"])
	}
	if errObj["retry_after"] != "42" {
		t.Fatalf("retry_after = %v, want 42", errObj["retry_after"])
	}
}

func TestAPIErrorSurfacesProviderBody(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {
			status: 400,
			body:   `{"description":"Validation failed","errors":[{"field":"email","message":"There is no contact matching the given email","code":"invalid_value"}]}`,
		},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "ticket", "create", "--subject", "s", "--description", "d", "--email", "nobody@acme.com")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errStr, "Validation failed") || !strings.Contains(errStr, "invalid_value") {
		t.Fatalf("stderr should carry provider body verbatim: %q", errStr)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	code, _, _ := run(t, nil, "ticket", "frobnicate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}
