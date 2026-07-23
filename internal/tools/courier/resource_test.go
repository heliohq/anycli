package courier

import (
	"net/http"
	"testing"
)

func TestMessageGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"req-1","status":"delivered"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "get", "req-1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/messages/req-1" {
		t.Fatalf("request = %s %s, want GET /messages/req-1", got.Method, got.Path)
	}
}

func TestMessageListFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[],"paging":{"more":false}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "list",
		"--status", "DELIVERED", "--recipient", "u1", "--notification", "n1",
		"--list", "list_1", "--tags", "a,b", "--cursor", "c1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/messages" {
		t.Fatalf("path = %q, want /messages", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("status") != "DELIVERED" || q.Get("recipient") != "u1" ||
		q.Get("notification") != "n1" || q.Get("list") != "list_1" ||
		q.Get("tags") != "a,b" || q.Get("cursor") != "c1" {
		t.Fatalf("query = %q, missing expected filters", got.Query)
	}
}

func TestMessageHistory(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "history", "req-1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/messages/req-1/history" {
		t.Fatalf("path = %q, want /messages/req-1/history", got.Path)
	}
}

func TestMessageCancel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"req-1","status":"CANCELED"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "cancel", "req-1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/messages/req-1/cancel" {
		t.Fatalf("request = %s %s, want POST /messages/req-1/cancel", got.Method, got.Path)
	}
	if len(got.Body) != 0 {
		t.Fatalf("cancel body = %q, want empty", got.Body)
	}
}

func TestListList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"paging":{"more":false}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "list", "--cursor", "c1", "--pattern", "abc*")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/lists" {
		t.Fatalf("path = %q, want /lists", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("cursor") != "c1" || q.Get("pattern") != "abc*" {
		t.Fatalf("query = %q, want cursor+pattern", got.Query)
	}
}

func TestListGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"list_1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "get", "list_1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/lists/list_1" {
		t.Fatalf("path = %q, want /lists/list_1", got.Path)
	}
}

func TestListSubscribe(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "subscribe", "list_1", "u1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/lists/list_1/subscriptions/u1" {
		t.Fatalf("request = %s %s, want PUT /lists/list_1/subscriptions/u1", got.Method, got.Path)
	}
}

func TestListUnsubscribe(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "list", "unsubscribe", "list_1", "u1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/lists/list_1/subscriptions/u1" {
		t.Fatalf("request = %s %s, want DELETE /lists/list_1/subscriptions/u1", got.Method, got.Path)
	}
}

func TestAudienceListAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"paging":{"more":false}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "audience", "list", "--cursor", "c1"); code != 0 {
		t.Fatalf("list exit = %d, want 0", code)
	}
	if got.Path != "/audiences" || parseQuery(t, got.Query).Get("cursor") != "c1" {
		t.Fatalf("audience list request wrong: %s?%s", got.Path, got.Query)
	}

	if code, _, _ := run(t, srv, "audience", "get", "aud_1"); code != 0 {
		t.Fatalf("get exit = %d, want 0", code)
	}
	if got.Path != "/audiences/aud_1" {
		t.Fatalf("audience get path = %q, want /audiences/aud_1", got.Path)
	}
}

func TestProfileGetAndSubscriptions(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"profile":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "profile", "get", "u1"); code != 0 {
		t.Fatalf("get exit = %d, want 0", code)
	}
	if got.Path != "/profiles/u1" {
		t.Fatalf("profile get path = %q, want /profiles/u1", got.Path)
	}

	if code, _, _ := run(t, srv, "profile", "subscriptions", "u1", "--cursor", "c1"); code != 0 {
		t.Fatalf("subscriptions exit = %d, want 0", code)
	}
	if got.Path != "/profiles/u1/lists" || parseQuery(t, got.Query).Get("cursor") != "c1" {
		t.Fatalf("profile subscriptions request wrong: %s?%s", got.Path, got.Query)
	}
}

func TestBrandListAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[],"paging":{"more":false}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "brand", "list", "--cursor", "c1"); code != 0 {
		t.Fatalf("list exit = %d, want 0", code)
	}
	if got.Path != "/brands" {
		t.Fatalf("brand list path = %q, want /brands", got.Path)
	}
	if code, _, _ := run(t, srv, "brand", "get", "brand_1"); code != 0 {
		t.Fatalf("get exit = %d, want 0", code)
	}
	if got.Path != "/brands/brand_1" {
		t.Fatalf("brand get path = %q, want /brands/brand_1", got.Path)
	}
}

func TestAutomationInvoke(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted, `{"runId":"run_1"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "automation", "invoke",
		"--automation", `{"steps":[{"action":"send"}]}`,
		"--recipient", "u1", "--data", `{"k":"v"}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/automations/invoke" {
		t.Fatalf("request = %s %s, want POST /automations/invoke", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	auto, ok := body["automation"].(map[string]any)
	if !ok || auto["steps"] == nil {
		t.Fatalf("automation body = %v, want automation.steps", body)
	}
	if body["recipient"] != "u1" {
		t.Fatalf("recipient = %v, want u1", body["recipient"])
	}
	if d, ok := body["data"].(map[string]any); !ok || d["k"] != "v" {
		t.Fatalf("data = %v, want {k:v}", body["data"])
	}
}

func TestAutomationInvokeMissingAutomationExits2(t *testing.T) {
	code, _, _ := run(t, nil, "automation", "invoke", "--recipient", "u1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (missing --automation)", code)
	}
}

func TestAutomationInvokeBadAutomationJSONExits2(t *testing.T) {
	code, _, _ := run(t, nil, "automation", "invoke", "--automation", `{bad`)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (bad --automation JSON)", code)
	}
}
