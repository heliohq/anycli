package brevo

import (
	"net/http"
	"testing"
)

func TestEmailSend_TransactionalBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"messageId":"<abc@relay>"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "email", "send",
		"--to", "jane@acme.com", "--to", "bob@acme.com",
		"--sender-email", "noreply@myco.com", "--sender-name", "MyCo",
		"--subject", "Summary", "--html", "<p>Hi</p>", "--text", "Hi")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/smtp/email" {
		t.Errorf("request = %s %s, want POST /smtp/email", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	to, ok := body["to"].([]any)
	if !ok || len(to) != 2 {
		t.Fatalf("to = %v, want two recipients", body["to"])
	}
	first := to[0].(map[string]any)
	if first["email"] != "jane@acme.com" {
		t.Errorf("to[0].email = %v", first["email"])
	}
	sender, ok := body["sender"].(map[string]any)
	if !ok || sender["email"] != "noreply@myco.com" || sender["name"] != "MyCo" {
		t.Errorf("sender = %v, want {email,name}", body["sender"])
	}
	if body["subject"] != "Summary" || body["htmlContent"] != "<p>Hi</p>" || body["textContent"] != "Hi" {
		t.Errorf("body = %v", body)
	}
	if got.APIKey != "key-123" {
		t.Errorf("api-key = %q", got.APIKey)
	}
	if stdout == "" {
		t.Error("want messageId passthrough on stdout")
	}
}

func TestEmailSend_SenderIDAndTemplate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"messageId":"<x>"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--to", "jane@acme.com", "--sender-id", "12",
		"--template-id", "7", "--params-json", `{"NAME":"Jane"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	sender, ok := body["sender"].(map[string]any)
	if !ok || sender["id"] != float64(12) {
		t.Errorf("sender = %v, want {id:12}", body["sender"])
	}
	if _, hasEmail := sender["email"]; hasEmail {
		t.Errorf("sender should carry id only, got %v", sender)
	}
	if body["templateId"] != float64(7) {
		t.Errorf("templateId = %v, want 7", body["templateId"])
	}
	params, ok := body["params"].(map[string]any)
	if !ok || params["NAME"] != "Jane" {
		t.Errorf("params = %v", body["params"])
	}
}

func TestEmailSend_ToJSONOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"messageId":"<x>"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--to-json", `[{"email":"vip@acme.com","name":"VIP"}]`,
		"--sender-email", "n@myco.com", "--subject", "S", "--html", "<p>x</p>")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	to := body["to"].([]any)
	if len(to) != 1 || to[0].(map[string]any)["name"] != "VIP" {
		t.Errorf("to = %v, want to-json override", body["to"])
	}
}

func TestEmailSend_InvalidParamsJSON_Exit2(t *testing.T) {
	srv := newServer(t, http.StatusCreated, `{}`, new(capturedRequest))
	defer srv.Close()

	code, _, stderr := run(t, srv, "email", "send",
		"--to", "j@acme.com", "--sender-email", "n@myco.com",
		"--subject", "S", "--html", "<p>x</p>", "--params-json", `{not json`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr == "" {
		t.Error("want a validation error on stderr")
	}
}

func TestEmailSend_CCBCCReplyTo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"messageId":"<x>"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "send",
		"--to", "j@acme.com", "--sender-email", "n@myco.com",
		"--subject", "S", "--html", "<p>x</p>",
		"--cc", "cc@acme.com", "--bcc", "bcc@acme.com", "--reply-to", "reply@myco.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	if cc, ok := body["cc"].([]any); !ok || cc[0].(map[string]any)["email"] != "cc@acme.com" {
		t.Errorf("cc = %v", body["cc"])
	}
	if bcc, ok := body["bcc"].([]any); !ok || bcc[0].(map[string]any)["email"] != "bcc@acme.com" {
		t.Errorf("bcc = %v", body["bcc"])
	}
	replyTo, ok := body["replyTo"].(map[string]any)
	if !ok || replyTo["email"] != "reply@myco.com" {
		t.Errorf("replyTo = %v", body["replyTo"])
	}
}
