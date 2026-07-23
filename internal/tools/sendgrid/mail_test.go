package sendgrid

import (
	"net/http"
	"strings"
	"testing"
)

// TestMailSend_202EmptyBodyXMessageID pins the central quirk: a 202 with an
// EMPTY body is success, and the tracking id comes from the X-Message-Id header,
// not the body. The handler must synthesize the acceptance object.
func TestMailSend_202EmptyBodyXMessageID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{
		status:  http.StatusAccepted,
		headers: map[string]string{"X-Message-Id": "msg-abc123"},
	}, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "mail", "send",
		"--to", "rcpt@example.com", "--from", "sender@example.com",
		"--subject", "Hi", "--text", "hello there")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/mail/send" {
		t.Errorf("request = %s %s, want POST /mail/send", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"status":"accepted"`) {
		t.Errorf("stdout = %q, want synthesized acceptance", stdout)
	}
	if !strings.Contains(stdout, `"message_id":"msg-abc123"`) {
		t.Errorf("stdout = %q, want the X-Message-Id echoed", stdout)
	}

	body := decodeBody(t, got.Body)
	personalizations, ok := body["personalizations"].([]any)
	if !ok || len(personalizations) != 1 {
		t.Fatalf("personalizations = %v, want one entry", body["personalizations"])
	}
	p := personalizations[0].(map[string]any)
	to := p["to"].([]any)
	if to[0].(map[string]any)["email"] != "rcpt@example.com" {
		t.Errorf("to = %v, want rcpt@example.com", to)
	}
	from := body["from"].(map[string]any)
	if from["email"] != "sender@example.com" {
		t.Errorf("from = %v, want sender@example.com", from)
	}
	if body["subject"] != "Hi" {
		t.Errorf("subject = %v, want Hi", body["subject"])
	}
	content := body["content"].([]any)
	first := content[0].(map[string]any)
	if first["type"] != "text/plain" || first["value"] != "hello there" {
		t.Errorf("content[0] = %v, want text/plain body", first)
	}
}

func TestMailSend_TemplateWithDynamicData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusAccepted, headers: map[string]string{"X-Message-Id": "m1"}}, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "mail", "send",
		"--to", "u@example.com", "--from", "s@example.com",
		"--template-id", "d-123", "--data", `{"first_name":"Ada"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	body := decodeBody(t, got.Body)
	if body["template_id"] != "d-123" {
		t.Errorf("template_id = %v, want d-123", body["template_id"])
	}
	// subject/content omitted for a template send.
	if _, ok := body["subject"]; ok {
		t.Errorf("subject should be omitted for a template send, body=%v", body)
	}
	p := body["personalizations"].([]any)[0].(map[string]any)
	data := p["dynamic_template_data"].(map[string]any)
	if data["first_name"] != "Ada" {
		t.Errorf("dynamic_template_data = %v, want first_name=Ada", data)
	}
}

func TestMailSend_JSONBodyEscapeHatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusAccepted, headers: map[string]string{"X-Message-Id": "m2"}}, &got)
	defer srv.Close()

	raw := `{"personalizations":[{"to":[{"email":"a@b.com"}]}],"from":{"email":"c@d.com"},"subject":"S","content":[{"type":"text/plain","value":"v"}]}`
	code, _, stderr := run(t, srv, "mail", "send", "--json-body", raw)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	body := decodeBody(t, got.Body)
	if body["subject"] != "S" {
		t.Errorf("subject = %v, want S from escape hatch", body["subject"])
	}
}

func TestMailSend_ValidationErrors(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusAccepted}, &got)
	defer srv.Close()

	// Missing --from.
	code, _, stderr := run(t, srv, "mail", "send", "--to", "x@y.com", "--subject", "S", "--text", "t")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--from") {
		t.Errorf("stderr = %q, want a --from validation error", stderr)
	}
}

func TestMailSend_ForbiddenIsNotCredentialReject(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusForbidden, body: `{"errors":[{"message":"from address does not match a verified Sender Identity"}]}`}, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "mail", "send",
		"--to", "x@y.com", "--from", "unverified@z.com", "--subject", "S", "--text", "t")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("a 403 verified-sender error must NOT reject the credential")
	}
	if !strings.Contains(stderr, "verified Sender Identity") {
		t.Errorf("stderr = %q, want the provider 403 message", stderr)
	}
}
