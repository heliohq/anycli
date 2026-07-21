package freshdesk

import (
	"net/http"
	"strings"
	"testing"
)

func TestTicketList_FiltersAndPaging(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":1}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "ticket", "list",
		"--filter", "new_and_my_open", "--company-id", "55", "--updated-since", "2026-01-01T00:00:00Z",
		"--order-by", "updated_at", "--order-type", "desc", "--include", "requester,stats",
		"--page", "2", "--per-page", "50")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/tickets" {
		t.Errorf("request = %s %s, want GET /tickets", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	checks := map[string]string{
		"filter": "new_and_my_open", "company_id": "55", "updated_since": "2026-01-01T00:00:00Z",
		"order_by": "updated_at", "order_type": "desc", "include": "requester,stats",
		"page": "2", "per_page": "50",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("query[%s] = %q, want %q", k, q.Get(k), want)
		}
	}
	if !strings.Contains(stdout, `"id":1`) {
		t.Errorf("stdout = %q, want JSON passthrough", stdout)
	}
}

func TestTicketList_NoStatusPriorityParams(t *testing.T) {
	// Divergence from DESIGN: GET /tickets does not accept status/priority
	// filters (search-only). The list command must not expose them.
	var out, errBuf capturedRequest
	_ = errBuf
	srv := newServer(t, http.StatusOK, `[]`, &out)
	defer srv.Close()
	code, _, stderr := run(t, srv, "ticket", "list", "--status", "2")
	if code == 0 {
		t.Errorf("expected non-zero for unknown --status flag on ticket list")
	}
	if !strings.Contains(stderr, "unknown flag") && !strings.Contains(stderr, "status") {
		t.Errorf("stderr = %q, want unknown-flag error for --status", stderr)
	}
}

func TestTicketGet_Include(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":7}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "get", "--id", "7", "--include", "conversations,requester")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/tickets/7" {
		t.Errorf("path = %q, want /tickets/7", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("include") != "conversations,requester" {
		t.Errorf("include = %q, want conversations,requester", q.Get("include"))
	}
}

func TestTicketCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "create",
		"--subject", "Help", "--description", "<b>hi</b>", "--email", "jane@acme.com",
		"--priority", "3", "--status", "2", "--group-id", "12", "--responder-id", "34",
		"--tags", "vip,billing", "--cc", "boss@acme.com",
		"--custom-fields", `{"cf_region":"emea"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/tickets" {
		t.Errorf("request = %s %s, want POST /tickets", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["subject"] != "Help" || body["email"] != "jane@acme.com" {
		t.Errorf("body subject/email wrong: %v", body)
	}
	if body["priority"] != float64(3) || body["status"] != float64(2) {
		t.Errorf("priority/status not numeric: %v %v", body["priority"], body["status"])
	}
	if body["group_id"] != float64(12) || body["responder_id"] != float64(34) {
		t.Errorf("group/responder not numeric: %v", body)
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "vip" {
		t.Errorf("tags wrong: %v", body["tags"])
	}
	cc, ok := body["cc_emails"].([]any)
	if !ok || len(cc) != 1 || cc[0] != "boss@acme.com" {
		t.Errorf("cc_emails wrong: %v", body["cc_emails"])
	}
	cf, ok := body["custom_fields"].(map[string]any)
	if !ok || cf["cf_region"] != "emea" {
		t.Errorf("custom_fields wrong: %v", body["custom_fields"])
	}
}

func TestTicketUpdate_Method(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "update", "--id", "9", "--status", "4", "--tags", "resolved")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/tickets/9" {
		t.Errorf("request = %s %s, want PUT /tickets/9", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["status"] != float64(4) {
		t.Errorf("status = %v, want 4", body["status"])
	}
	if _, ok := body["tags"]; !ok {
		t.Errorf("tags should be set when --tags provided: %v", body)
	}
}

func TestTicketSearch_QuotesQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"total":0,"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "search", "--query", "status:2 AND priority:4", "--page", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/search/tickets" {
		t.Errorf("path = %q, want /search/tickets", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("query") != `"status:2 AND priority:4"` {
		t.Errorf("query = %q, want the double-quoted form", q.Get("query"))
	}
	if q.Get("page") != "3" {
		t.Errorf("page = %q, want 3", q.Get("page"))
	}
}

func TestTicketSearch_PreQuotedNotDoubled(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "search", "--query", `"status:5"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if q := parseQuery(t, got.Query); q.Get("query") != `"status:5"` {
		t.Errorf("query = %q, want single-quoted wrap (not doubled)", q.Get("query"))
	}
}

func TestTicketReply(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":100}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "reply", "--id", "5", "--body", "thanks", "--cc", "a@x.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/tickets/5/reply" {
		t.Errorf("request = %s %s, want POST /tickets/5/reply", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["body"] != "thanks" {
		t.Errorf("body = %v, want thanks", body["body"])
	}
	if cc, ok := body["cc_emails"].([]any); !ok || cc[0] != "a@x.com" {
		t.Errorf("cc_emails = %v", body["cc_emails"])
	}
}

func TestTicketNote_PrivateByDefault(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":101}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "note", "--id", "5", "--body", "internal")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/tickets/5/notes" {
		t.Errorf("path = %q, want /tickets/5/notes", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["private"] != true {
		t.Errorf("private = %v, want true (default)", body["private"])
	}
}

func TestTicketNote_PublicOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":102}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "note", "--id", "5", "--body", "public note", "--public", "--notify", "agent@acme.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	if body["private"] != false {
		t.Errorf("private = %v, want false (--public)", body["private"])
	}
	if n, ok := body["notify_emails"].([]any); !ok || n[0] != "agent@acme.com" {
		t.Errorf("notify_emails = %v", body["notify_emails"])
	}
}

func TestTicketConversations(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":1}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "conversations", "--id", "5", "--per-page", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/tickets/5/conversations" {
		t.Errorf("path = %q, want /tickets/5/conversations", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("per_page") != "10" {
		t.Errorf("per_page = %q, want 10", q.Get("per_page"))
	}
}
