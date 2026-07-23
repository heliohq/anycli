package resend

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmailSend_MinimalBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "email", "send",
		"--from", "onboarding@example.com", "--to", "user@dest.com",
		"--subject", "hi", "--text", "hello")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/emails" {
		t.Errorf("request = %s %s, want POST /emails", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["from"] != "onboarding@example.com" || body["subject"] != "hi" || body["text"] != "hello" {
		t.Errorf("body = %v", body)
	}
	// Single --to renders as a scalar string (Resend accepts string|array).
	if body["to"] != "user@dest.com" {
		t.Errorf("to = %v, want scalar string user@dest.com", body["to"])
	}
	if !strings.Contains(stdout, `"id"`) {
		t.Errorf("stdout = %q, want passthrough JSON", stdout)
	}
}

func TestEmailSend_MultipleRecipientsArray(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--from", "a@example.com", "--to", "one@d.com", "--to", "two@d.com",
		"--subject", "hi", "--html", "<p>hi</p>",
		"--cc", "c@d.com", "--bcc", "b@d.com", "--reply-to", "r@d.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	to, ok := body["to"].([]any)
	if !ok || len(to) != 2 {
		t.Fatalf("to = %v, want 2-element array", body["to"])
	}
	if body["html"] != "<p>hi</p>" {
		t.Errorf("html = %v", body["html"])
	}
	if body["cc"] != "c@d.com" || body["bcc"] != "b@d.com" {
		t.Errorf("cc/bcc = %v / %v", body["cc"], body["bcc"])
	}
	// reply_to is the wire field name (flag is --reply-to).
	if body["reply_to"] != "r@d.com" {
		t.Errorf("reply_to = %v", body["reply_to"])
	}
}

func TestEmailSend_ScheduledAndStructuredFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--from", "a@example.com", "--to", "d@d.com", "--subject", "s", "--text", "t",
		"--scheduled-at", "in 1 min",
		"--attachments", `[{"filename":"x.pdf","content":"Zm9v"}]`,
		"--tags", `[{"name":"env","value":"prod"}]`,
		"--headers", `{"X-Entity-Ref-ID":"abc"}`,
		"--idempotency-key", "order-123")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	if body["scheduled_at"] != "in 1 min" {
		t.Errorf("scheduled_at = %v", body["scheduled_at"])
	}
	if _, ok := body["attachments"].([]any); !ok {
		t.Errorf("attachments = %v, want array passthrough", body["attachments"])
	}
	if _, ok := body["tags"].([]any); !ok {
		t.Errorf("tags = %v, want array passthrough", body["tags"])
	}
	if _, ok := body["headers"].(map[string]any); !ok {
		t.Errorf("headers = %v, want object passthrough", body["headers"])
	}
	if got.IdemKey != "order-123" {
		t.Errorf("Idempotency-Key header = %q, want order-123", got.IdemKey)
	}
}

func TestEmailSend_InvalidStructuredJSONIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "email", "send",
		"--from", "a@example.com", "--to", "d@d.com", "--subject", "s", "--text", "t",
		"--attachments", `{not json`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want JSON validation error", stderr)
	}
}

func TestEmailBatch_ArrayBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"e1"},{"id":"e2"}]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "batch", "--emails",
		`[{"from":"a@example.com","to":"x@d.com","subject":"s1","text":"t"},{"from":"a@example.com","to":"y@d.com","subject":"s2","text":"t"}]`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/emails/batch" {
		t.Errorf("request = %s %s, want POST /emails/batch", got.Method, got.Path)
	}
	arr := decodeArrayBody(t, got.Body)
	if len(arr) != 2 {
		t.Errorf("batch body = %v, want 2 emails", arr)
	}
}

func TestEmailGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1","last_event":"delivered"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "get", "e1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/emails/e1" {
		t.Errorf("request = %s %s, want GET /emails/e1", got.Method, got.Path)
	}
}

func TestEmailUpdate_Reschedule(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "update", "e1", "--scheduled-at", "2030-01-01T09:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/emails/e1" {
		t.Errorf("request = %s %s, want PATCH /emails/e1", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["scheduled_at"] != "2030-01-01T09:00:00Z" {
		t.Errorf("scheduled_at = %v", body["scheduled_at"])
	}
}

func TestEmailCancel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "cancel", "e1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/emails/e1/cancel" {
		t.Errorf("request = %s %s, want POST /emails/e1/cancel", got.Method, got.Path)
	}
}
