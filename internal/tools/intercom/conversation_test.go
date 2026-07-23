package intercom

import (
	"net/http"
	"testing"
)

func TestConversationList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation.list"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "list", "--per-page", "30", "--starting-after", "cur1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/conversations" {
		t.Errorf("request = %s %s, want GET /conversations", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("per_page") != "30" || q.Get("starting_after") != "cur1" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestConversationReply_CommentAutoResolvesAdmin(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/me":                     {status: http.StatusOK, response: `{"type":"admin","id":"814860"}`},
		"/conversations/42/reply": {status: http.StatusOK, response: `{"type":"conversation","id":"42"}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "reply", "--id", "42", "--body", "Hello there")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, ok := captured["/me"]; !ok {
		t.Fatal("expected GET /me for admin auto-resolution")
	}
	req := captured["/conversations/42/reply"]
	body := decodeBody(t, req.Body)
	if body["message_type"] != "comment" {
		t.Errorf("message_type = %v, want comment", body["message_type"])
	}
	if body["type"] != "admin" {
		t.Errorf("type = %v, want admin", body["type"])
	}
	if body["admin_id"] != "814860" {
		t.Errorf("admin_id = %v, want auto-resolved 814860", body["admin_id"])
	}
	if body["body"] != "Hello there" {
		t.Errorf("body = %v", body["body"])
	}
}

func TestConversationNote_MessageTypeNote(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation"}`, &got)
	defer srv.Close()

	// Explicit --admin-id skips the /me lookup entirely (single-server ok).
	code, _, _ := run(t, srv, "conversation", "note", "--id", "42", "--body", "internal", "--admin-id", "999")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/conversations/42/reply" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "note" {
		t.Errorf("message_type = %v, want note", body["message_type"])
	}
	if body["admin_id"] != "999" {
		t.Errorf("admin_id = %v, want explicit 999 (no /me lookup)", body["admin_id"])
	}
}

func TestConversationClose(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "close", "--id", "42", "--admin-id", "1", "--body", "done")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/conversations/42/parts" {
		t.Errorf("request = %s %s, want POST /conversations/42/parts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "close" || body["type"] != "admin" || body["admin_id"] != "1" {
		t.Errorf("body = %v", body)
	}
	if body["body"] != "done" {
		t.Errorf("close body = %v, want done", body["body"])
	}
}

func TestConversationSnooze(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "snooze", "--id", "42", "--admin-id", "1", "--snoozed-until", "1501512795")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "snoozed" {
		t.Errorf("message_type = %v, want snoozed", body["message_type"])
	}
	if body["snoozed_until"] != "1501512795" {
		t.Errorf("snoozed_until = %v", body["snoozed_until"])
	}
}

func TestConversationAssign(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "assign", "--id", "42", "--admin-id", "1", "--assignee-id", "530165")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "assignment" || body["type"] != "admin" {
		t.Errorf("body = %v", body)
	}
	if body["assignee_id"] != "530165" {
		t.Errorf("assignee_id = %v, want 530165", body["assignee_id"])
	}
}

func TestConversationTag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"tag"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "tag", "--id", "42", "--tag-id", "t1", "--admin-id", "1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/conversations/42/tags" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["id"] != "t1" || body["admin_id"] != "1" {
		t.Errorf("body = %v", body)
	}
}

func TestConversationUntag_DeleteWithAdminBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"tag"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "untag", "--id", "42", "--tag-id", "t1", "--admin-id", "1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/conversations/42/tags/t1" {
		t.Errorf("request = %s %s, want DELETE /conversations/42/tags/t1", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["admin_id"] != "1" {
		t.Errorf("body = %v, want admin_id", body)
	}
}

func TestConversationSearch_ConvenienceFiltersCompile(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation.list"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "search", "--state", "open", "--updated-since", "1693782000", "--per-page", "10")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/conversations/search" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	q, ok := body["query"].(map[string]any)
	if !ok || q["operator"] != "AND" {
		t.Fatalf("query = %v, want AND group", body["query"])
	}
	vals, ok := q["value"].([]any)
	if !ok || len(vals) != 2 {
		t.Fatalf("value = %v, want 2 filters", q["value"])
	}
	p, ok := body["pagination"].(map[string]any)
	if !ok || p["per_page"].(float64) != 10 {
		t.Errorf("pagination = %v, want per_page 10", body["pagination"])
	}
}

func TestConversationSearch_RawQueryPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"conversation.list"}`, &got)
	defer srv.Close()

	raw := `{"field":"open","operator":"=","value":true}`
	code, _, _ := run(t, srv, "conversation", "search", "--query", raw)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	q := body["query"].(map[string]any)
	if q["field"] != "open" || q["operator"] != "=" || q["value"] != true {
		t.Errorf("query = %v, want raw passthrough", q)
	}
}

func TestConversationSearch_MutuallyExclusive_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "conversation", "search", "--query", `{}`, "--state", "open")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
	if got.Method != "" {
		t.Error("no HTTP request should be made on a usage error")
	}
	_ = stderr
}
