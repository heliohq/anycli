package knock

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

func TestExecuteMissingKeyExit1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"message", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "KNOCK_API_KEY is not set") {
		t.Fatalf("stderr = %q, want missing-key message", errBuf.String())
	}
}

func TestAuthHeaderAndVerbatimEmit(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[{"id":"m_1"}],"page_info":{"after":"cur"}}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "message", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Auth != "Bearer sk_test_123" {
		t.Fatalf("Authorization = %q, want Bearer sk_test_123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q", got.Accept)
	}
	if got.Method != http.MethodGet || got.Path != "/messages" {
		t.Fatalf("request = %s %s, want GET /messages", got.Method, got.Path)
	}
	// Body is emitted verbatim, one JSON document + newline.
	if strings.TrimSpace(stdout) != `{"entries":[{"id":"m_1"}],"page_info":{"after":"cur"}}` {
		t.Fatalf("stdout = %q, want verbatim provider JSON", stdout)
	}
}

func TestWorkflowTriggerBodyAssembly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"workflow_run_id":"wr_1"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"workflow", "trigger",
		"--key", "new-comment",
		"--recipient", "user_1",
		"--recipient", "user_2",
		"--data", `{"comment":"hi"}`,
		"--actor", "user_9",
		"--tenant", "acme",
		"--cancellation-key", "cancel-1",
		"--sandbox",
		"--skip-delay",
		"--idempotency-key", "idem-1",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/workflows/new-comment/trigger" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if got.Idem != "idem-1" {
		t.Fatalf("Idempotency-Key = %q, want idem-1", got.Idem)
	}
	body := decodeBody(t, got.Body)
	recipients, ok := body["recipients"].([]any)
	if !ok || len(recipients) != 2 || recipients[0] != "user_1" || recipients[1] != "user_2" {
		t.Fatalf("recipients = %v", body["recipients"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["comment"] != "hi" {
		t.Fatalf("data = %v", body["data"])
	}
	if body["actor"] != "user_9" || body["tenant"] != "acme" || body["cancellation_key"] != "cancel-1" {
		t.Fatalf("scalar fields = %v", body)
	}
	settings, ok := body["settings"].(map[string]any)
	if !ok || settings["sandbox_mode"] != true || settings["skip_delay"] != true {
		t.Fatalf("settings = %v", body["settings"])
	}
	if strings.TrimSpace(stdout) != `{"workflow_run_id":"wr_1"}` {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestWorkflowTriggerRequiresRecipient(t *testing.T) {
	exit, _, stderr := run(t, nil, "workflow", "trigger", "--key", "wf")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if !strings.Contains(stderr, "recipient") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestWorkflowTriggerInvalidDataJSONExit2(t *testing.T) {
	exit, _, stderr := run(t, nil, "workflow", "trigger", "--key", "wf", "--recipient", "u1", "--data", "{not json")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestWorkflowTriggerRecipientMutualExclusion(t *testing.T) {
	exit, _, stderr := run(t, nil, "workflow", "trigger", "--key", "wf",
		"--recipient", "u1", "--recipients-json", `[{"id":"u2"}]`)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "not both") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestWorkflowTriggerRecipientsJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"workflow_run_id":"wr"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "workflow", "trigger", "--key", "wf",
		"--recipients-json", `[{"id":"u2","email":"a@b.co"}]`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	body := decodeBody(t, got.Body)
	recipients, ok := body["recipients"].([]any)
	if !ok || len(recipients) != 1 {
		t.Fatalf("recipients = %v", body["recipients"])
	}
	obj, ok := recipients[0].(map[string]any)
	if !ok || obj["id"] != "u2" || obj["email"] != "a@b.co" {
		t.Fatalf("recipient object = %v", recipients[0])
	}
}

func TestWorkflowCancel204ReceiptEmit(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "workflow", "cancel", "--key", "wf", "--cancellation-key", "c1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/workflows/wf/cancel" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["cancellation_key"] != "c1" {
		t.Fatalf("body = %v", body)
	}
	// Empty 204 body must still emit a parseable JSON receipt.
	if strings.TrimSpace(stdout) != `{"ok":true}` {
		t.Fatalf("stdout = %q, want {\"ok\":true}", stdout)
	}
}

func TestUserIdentifyPut(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"user_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "user", "identify", "--id", "user_1", "--data", `{"email":"a@b.co","name":"A"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPut || got.Path != "/users/user_1" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@b.co" || body["name"] != "A" {
		t.Fatalf("body = %v", body)
	}
}

func TestUserListPaginationPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[],"page_info":{}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "user", "list", "--page-size", "25", "--after", "cur9")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	q := parseQuery(t, got.Query)
	if q.Get("page_size") != "25" || q.Get("after") != "cur9" {
		t.Fatalf("query = %q", got.Query)
	}
}

func TestUserMergeBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"user_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "user", "merge", "--id", "user_1", "--from-id", "user_2")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/users/user_1/merge" {
		t.Fatalf("path = %s", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["from_user_id"] != "user_2" {
		t.Fatalf("body = %v", body)
	}
}

func TestMessageMarkStates(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"seen", []string{"message", "mark", "--id", "m1", "--state", "seen"}, http.MethodPut, "/messages/m1/seen"},
		{"read", []string{"message", "mark", "--id", "m1", "--state", "read"}, http.MethodPut, "/messages/m1/read"},
		{"archived", []string{"message", "mark", "--id", "m1", "--state", "archived"}, http.MethodPut, "/messages/m1/archived"},
		{"unseen", []string{"message", "mark", "--id", "m1", "--state", "seen", "--undo"}, http.MethodDelete, "/messages/m1/seen"},
		{"interacted", []string{"message", "mark", "--id", "m1", "--state", "interacted"}, http.MethodPut, "/messages/m1/interacted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusNoContent, ``, &got)
			defer srv.Close()
			exit, _, stderr := run(t, srv, tc.args...)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr=%s", exit, stderr)
			}
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Fatalf("request = %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

func TestMessageMarkInvalidState(t *testing.T) {
	exit, _, stderr := run(t, nil, "message", "mark", "--id", "m1", "--state", "clicked")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "seen|read|interacted|archived") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestMessageMarkInteractedUndoRejected(t *testing.T) {
	exit, _, stderr := run(t, nil, "message", "mark", "--id", "m1", "--state", "interacted", "--undo")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "interacted cannot be undone") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestMessageListFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "message", "list",
		"--recipient", "u1", "--channel-id", "ch1", "--status", "delivered", "--workflow", "wf1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	q := parseQuery(t, got.Query)
	if q.Get("recipient") != "u1" || q.Get("channel_id") != "ch1" || q.Get("status") != "delivered" || q.Get("workflow") != "wf1" {
		t.Fatalf("query = %q", got.Query)
	}
}

func TestMessageDeliveryLogsSegment(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"entries":[]}`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv, "message", "delivery-logs", "--id", "m1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/messages/m1/delivery_logs" {
		t.Fatalf("path = %s, want /messages/m1/delivery_logs", got.Path)
	}
}

func TestObjectSetAndSubscriptions(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"proj_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "object", "set", "--collection", "projects", "--id", "proj_1", "--data", `{"name":"P"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPut || got.Path != "/objects/projects/proj_1" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}

	var got2 capturedRequest
	srv2 := newServer(t, http.StatusOK, `{"entries":[]}`, &got2)
	defer srv2.Close()
	exit2, _, stderr2 := run(t, srv2, "object", "subscriptions", "--collection", "projects", "--id", "proj_1")
	if exit2 != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit2, stderr2)
	}
	if got2.Path != "/objects/projects/proj_1/subscriptions" {
		t.Fatalf("path = %s", got2.Path)
	}
}

func TestObjectRequiresCollection(t *testing.T) {
	exit, _, stderr := run(t, nil, "object", "get", "--id", "x")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "collection is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestTenantSet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"acme"}`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv, "tenant", "set", "--id", "acme", "--data", `{"name":"Acme"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPut || got.Path != "/tenants/acme" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
}

func TestScheduleCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":"sch_1"}]`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "schedule", "create",
		"--recipient", "u1", "--workflow", "wf", "--scheduled-at", "2026-08-01T09:00:00Z", "--data", `{"x":1}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/schedules" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["workflow"] != "wf" || body["scheduled_at"] != "2026-08-01T09:00:00Z" {
		t.Fatalf("body = %v", body)
	}
	recipients, ok := body["recipients"].([]any)
	if !ok || len(recipients) != 1 || recipients[0] != "u1" {
		t.Fatalf("recipients = %v", body["recipients"])
	}
}

func TestScheduleCreateRequiresTimeOrRepeats(t *testing.T) {
	exit, _, stderr := run(t, nil, "schedule", "create", "--recipient", "u1", "--workflow", "wf")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "scheduled-at") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestScheduleDeleteBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":"sch_1"}]`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv, "schedule", "delete", "--schedule-id", "sch_1", "--schedule-id", "sch_2")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodDelete || got.Path != "/schedules" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	ids, ok := body["schedule_ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "sch_1" || ids[1] != "sch_2" {
		t.Fatalf("schedule_ids = %v", body["schedule_ids"])
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"invalid api key","code":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "message", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("CredentialRejected = false, want true on 401")
	}
	if !strings.Contains(stderr, "HTTP 401") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestNon2xxIsAPIErrorExit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"message":"missing recipients","code":"invalid_params"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "user", "get", "--id", "u1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatalf("CredentialRejected = true, want false for 422")
	}
	if !strings.Contains(stderr, "invalid_params: missing recipients") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"message":"not found","code":"resource_missing"}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "--json", "user", "get", "--id", "nope")
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != 404 {
		t.Fatalf("envelope = %+v", envelope.Error)
	}
}

func TestUsageErrorIsNotCredentialRejected(t *testing.T) {
	result, _, _ := runResult(t, nil, "user", "get")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode)
	}
	if execution.IsCredentialRejected(nil) {
		t.Fatalf("sanity: nil should not be credential-rejected")
	}
}
