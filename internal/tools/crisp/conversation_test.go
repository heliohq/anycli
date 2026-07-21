package crisp

import (
	"net/http"
	"testing"
)

// TestConversationList proves the page-suffixed path and default page number.
func TestConversationList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "list", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", got.Method)
	}
	if got.Path != "/website/wid-1/conversations/1" {
		t.Errorf("path = %q, want /website/wid-1/conversations/1", got.Path)
	}
}

// TestConversationListPageAndFilter proves --page and --filter-status shaping.
func TestConversationListPageAndFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "list", "--website", "wid-1", "--page", "3", "--filter-status", "resolved")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Path != "/website/wid-1/conversations/3" {
		t.Errorf("path = %q, want page 3", got.Path)
	}
	if got.Query != "filter_resolved=1" {
		t.Errorf("query = %q, want filter_resolved=1", got.Query)
	}
}

// TestConversationListBadFilter proves an unsupported status is a usage error.
func TestConversationListBadFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "conversation", "list", "--website", "wid-1", "--filter-status", "archived")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for a bad --filter-status", exit)
	}
	if got.Method != "" {
		t.Errorf("a network call was made; none expected for a usage error")
	}
}

// TestConversationGet proves the singular conversation path.
func TestConversationGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{"session_id":"s1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "get", "--session", "s1", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/website/wid-1/conversation/s1" {
		t.Errorf("got %s %s, want GET /website/wid-1/conversation/s1", got.Method, got.Path)
	}
}

// TestConversationGetMissingSession proves --session is required (usage error).
func TestConversationGetMissingSession(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "conversation", "get", "--website", "wid-1")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if got.Method != "" {
		t.Errorf("a network call was made; none expected")
	}
}

// TestConversationMessages proves the messages path and optional --before.
func TestConversationMessages(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "messages", "--session", "s1", "--website", "wid-1", "--before", "1700000000000")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Path != "/website/wid-1/conversation/s1/messages" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query != "timestamp_before=1700000000000" {
		t.Errorf("query = %q, want timestamp_before=1700000000000", got.Query)
	}
}

// TestConversationReply proves the POST message body defaults (text/operator/chat).
func TestConversationReply(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{"fingerprint":123}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "reply", "--session", "s1", "--text", "hello there", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/website/wid-1/conversation/s1/message" {
		t.Errorf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["type"] != "text" || body["from"] != "operator" || body["origin"] != "chat" {
		t.Errorf("body defaults wrong: %v", body)
	}
	if body["content"] != "hello there" {
		t.Errorf("content = %v, want 'hello there'", body["content"])
	}
}

// TestConversationReplyFromUser proves --from overrides the sender.
func TestConversationReplyFromUser(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "conversation", "reply", "--session", "s1", "--text", "hi", "--from", "user", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	body := decodeBody(t, got.Body)
	if body["from"] != "user" {
		t.Errorf("from = %v, want user", body["from"])
	}
}

// TestConversationReplyBadFrom proves an invalid --from is a usage error.
func TestConversationReplyBadFrom(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "conversation", "reply", "--session", "s1", "--text", "hi", "--from", "robot", "--website", "wid-1")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
}

// TestConversationState proves the state PATCH body and enum validation.
func TestConversationState(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "state", "--session", "s1", "--state", "resolved", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodPatch || got.Path != "/website/wid-1/conversation/s1/state" {
		t.Errorf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["state"] != "resolved" {
		t.Errorf("state = %v, want resolved", body["state"])
	}
}

// TestConversationStateBadEnum proves an invalid state is a usage error.
func TestConversationStateBadEnum(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "conversation", "state", "--session", "s1", "--state", "closed", "--website", "wid-1")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if got.Method != "" {
		t.Errorf("a network call was made; none expected")
	}
}

// TestConversationRouteByUUID proves a raw operator id skips the lookup.
func TestConversationRouteByUUID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "route", "--session", "s1", "--operator", "op-uuid-9", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodPatch || got.Path != "/website/wid-1/conversation/s1/routing" {
		t.Errorf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	assigned, ok := body["assigned"].(map[string]any)
	if !ok || assigned["user_id"] != "op-uuid-9" {
		t.Errorf("assigned = %v, want user_id op-uuid-9", body["assigned"])
	}
}

// TestConversationRouteByEmail proves an email is resolved via operators/list.
func TestConversationRouteByEmail(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/website/wid-1/operators/list":          {status: http.StatusOK, response: `{"error":false,"data":[{"type":"member","details":{"user_id":"op-42","email":"agent@example.com"}}]}`},
		"/website/wid-1/conversation/s1/routing": {status: http.StatusOK, response: `{"error":false,"data":{}}`},
	}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "route", "--session", "s1", "--operator", "agent@example.com", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if _, ok := captured["/website/wid-1/operators/list"]; !ok {
		t.Errorf("operators/list was not queried")
	}
	routing := captured["/website/wid-1/conversation/s1/routing"]
	body := decodeBody(t, routing.Body)
	assigned, _ := body["assigned"].(map[string]any)
	if assigned["user_id"] != "op-42" {
		t.Errorf("resolved user_id = %v, want op-42", assigned["user_id"])
	}
}

// TestConversationRouteEmailNotFound proves an unmatched email is exit 1.
func TestConversationRouteEmailNotFound(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/website/wid-1/operators/list": {status: http.StatusOK, response: `{"error":false,"data":[{"type":"member","details":{"user_id":"op-42","email":"other@example.com"}}]}`},
	}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "route", "--session", "s1", "--operator", "missing@example.com", "--website", "wid-1")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", exit, stderr)
	}
	if _, ok := captured["/website/wid-1/conversation/s1/routing"]; ok {
		t.Errorf("routing PATCH was sent despite an unmatched email")
	}
}
