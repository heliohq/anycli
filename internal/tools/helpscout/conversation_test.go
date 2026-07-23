package helpscout

import (
	"net/http"
	"strings"
	"testing"
)

func TestConversationList_QueryAndAuthAndPassthrough(t *testing.T) {
	var got capturedRequest
	hal := `{"_embedded":{"conversations":[{"id":1}]},"page":{"size":50,"number":1},"_links":{}}`
	srv := newServer(t, http.StatusOK, hal, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "conversation", "list",
		"--mailbox", "12", "--status", "active", "--tag", "vip",
		"--assigned-to", "99", "--modified-since", "2026-07-01T00:00:00Z",
		"--query", `modifiedAt:[NOW-1HOUR TO *]`, "--sort-field", "modifiedAt",
		"--sort-order", "desc", "--page", "2", "--embed-threads")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/conversations" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q", got.Auth)
	}
	q := parseQuery(t, got.Query)
	checks := map[string]string{
		"mailbox": "12", "status": "active", "tag": "vip", "assigned_to": "99",
		"modifiedSince": "2026-07-01T00:00:00Z", "query": "modifiedAt:[NOW-1HOUR TO *]",
		"sortField": "modifiedAt", "sortOrder": "desc", "page": "2", "embed": "threads",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("query[%s] = %q, want %q", k, q.Get(k), want)
		}
	}
	// HAL envelope passes through verbatim (+ newline).
	if strings.TrimSpace(stdout) != hal {
		t.Errorf("stdout = %q, want HAL passthrough", stdout)
	}
}

func TestConversationList_RejectsBadStatusEnum(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "conversation", "list", "--status", "bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call on a bad enum")
	}
	if !strings.Contains(stderr, "--status must be one of") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestConversationGet_EmbedThreads(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "get", "42", "--embed-threads")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/conversations/42" {
		t.Errorf("path = %s", got.Path)
	}
	if parseQuery(t, got.Query).Get("embed") != "threads" {
		t.Errorf("embed not set: %s", got.Query)
	}
}

func TestConversationCreate_BodyAndResourceIDReceipt(t *testing.T) {
	var got capturedRequest
	srv := newHeaderServer(t, http.StatusCreated, ``, map[string]string{"Resource-ID": "555"}, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "conversation", "create",
		"--mailbox", "12", "--subject", "Help", "--customer-email", "a@b.com",
		"--text", "hi there", "--tags", "vip,urgent")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/conversations" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["mailboxId"] != float64(12) {
		t.Errorf("mailboxId = %v, want numeric 12", body["mailboxId"])
	}
	if body["type"] != "email" {
		t.Errorf("type = %v, want defaulted email", body["type"])
	}
	if body["status"] != "active" {
		t.Errorf("status = %v, want defaulted active (always sent)", body["status"])
	}
	cust, _ := body["customer"].(map[string]any)
	if cust["email"] != "a@b.com" {
		t.Errorf("customer = %v", body["customer"])
	}
	threads, _ := body["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("threads = %v, want exactly one", body["threads"])
	}
	th, _ := threads[0].(map[string]any)
	if th["type"] != "customer" || th["text"] != "hi there" {
		t.Errorf("thread = %v", th)
	}
	tags, _ := body["tags"].([]any)
	if len(tags) != 2 || tags[0] != "vip" || tags[1] != "urgent" {
		t.Errorf("tags = %v", body["tags"])
	}
	// Empty-body 201 → receipt carrying the Resource-Id.
	rec := decodeBody(t, []byte(stdout))
	if rec["id"] != "555" || rec["status"] != "created" {
		t.Errorf("receipt = %s, want id=555 status=created", strings.TrimSpace(stdout))
	}
}

func TestConversationCreate_RequiresCustomer(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "conversation", "create",
		"--mailbox", "12", "--subject", "Help", "--text", "hi")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call when customer is missing")
	}
	if !strings.Contains(stderr, "customer-email") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestConversationUpdate_CompilesJSONPatchOps(t *testing.T) {
	captured := map[string]capturedRequest{}
	var seen [][]byte
	// The service issues one PATCH per op; record every body.
	srv := newMultiServer(t, map[string]routeHandler{
		"/conversations/7": {status: http.StatusNoContent, response: ``},
	}, captured)
	defer srv.Close()
	_ = seen

	code, stdout, stderr := run(t, srv, "conversation", "update", "7",
		"--status", "closed", "--assign-to", "88", "--subject", "New subj")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	req := captured["/conversations/7"]
	if req.Method != http.MethodPatch {
		t.Fatalf("method = %s, want PATCH", req.Method)
	}
	// Last op recorded is the assignTo replace; assert its shape.
	last := decodeBody(t, req.Body)
	if last["op"] != "replace" {
		t.Errorf("op = %v", last["op"])
	}
	rec := decodeBody(t, []byte(stdout))
	if rec["id"] != "7" || rec["status"] != "updated" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

// TestConversationUpdate_AcceptsOpenStatus locks in that `update --status open`
// is accepted client-side — the JSON-Patch /status set matches the reply set
// (Errors doc), so it compiles to a /status replace op rather than being
// rejected as a bad enum.
func TestConversationUpdate_AcceptsOpenStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "conversation", "update", "7", "--status", "open")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	body := decodeBody(t, got.Body)
	if body["op"] != "replace" || body["path"] != "/status" || body["value"] != "open" {
		t.Errorf("op = %v, want replace /status=open", body)
	}
}

func TestConversationUpdate_UnassignRemoveOp(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "conversation", "update", "7", "--unassign")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	body := decodeBody(t, got.Body)
	if body["op"] != "remove" || body["path"] != "/assignTo" {
		t.Errorf("op = %v", body)
	}
	if _, ok := body["value"]; ok {
		t.Errorf("remove op must not carry a value: %v", body)
	}
}

func TestConversationUpdate_MutuallyExclusiveAssign(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "conversation", "update", "7", "--assign-to", "1", "--unassign")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestConversationUpdate_NoFieldsIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "update", "7")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call with nothing to update")
	}
}

func TestConversationTag_ReplacesSet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "conversation", "tag", "7", "--tags", "a, b ,c")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/conversations/7/tags" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	tags, _ := body["tags"].([]any)
	if len(tags) != 3 || tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Errorf("tags = %v (whitespace should be trimmed)", body["tags"])
	}
	rec := decodeBody(t, []byte(stdout))
	if rec["status"] != "tagged" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

func TestConversationSnooze_PUTBodyWithBothFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "conversation", "snooze", "7", "--until", "2030-01-01T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != http.MethodPut || got.Path != "/conversations/7/snooze" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["snoozedUntil"] != "2030-01-01T00:00:00Z" {
		t.Errorf("snoozedUntil = %v", body["snoozedUntil"])
	}
	// The API requires unsnoozeOnCustomerReply; it must always be present and
	// default to true.
	if body["unsnoozeOnCustomerReply"] != true {
		t.Errorf("unsnoozeOnCustomerReply = %v, want default true", body["unsnoozeOnCustomerReply"])
	}
	rec := decodeBody(t, []byte(stdout))
	if rec["status"] != "snoozed" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

func TestConversationSnooze_UnsnoozeOnReplyFalse(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "snooze", "7",
		"--until", "2030-01-01T00:00:00Z", "--unsnooze-on-customer-reply=false")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if decodeBody(t, got.Body)["unsnoozeOnCustomerReply"] != false {
		t.Errorf("flag not honored: %s", got.Body)
	}
}

func TestConversationSnooze_RequiresUntil(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "snooze", "7")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call without --until")
	}
}

func TestConversationUnsnooze_Delete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "conversation", "unsnooze", "7")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/conversations/7/snooze" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	if decodeBody(t, []byte(stdout))["status"] != "unsnoozed" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}
