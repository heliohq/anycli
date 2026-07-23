package courier

import (
	"net/http"
	"testing"
)

func TestSendUserIDWithInlineContent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted, `{"requestId":"1-abc"}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv,
		"send", "--user-id", "u1", "--title", "Hi", "--body", "there")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/send" {
		t.Fatalf("request = %s %s, want POST /send", got.Method, got.Path)
	}
	if got.CType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got.CType)
	}
	body := decodeBody(t, got.Body)
	msg, ok := body["message"].(map[string]any)
	if !ok {
		t.Fatalf("body has no message object: %v", body)
	}
	to, ok := msg["to"].(map[string]any)
	if !ok || to["user_id"] != "u1" {
		t.Fatalf("message.to = %v, want {user_id: u1}", msg["to"])
	}
	content, ok := msg["content"].(map[string]any)
	if !ok || content["title"] != "Hi" || content["body"] != "there" {
		t.Fatalf("message.content = %v, want {title:Hi, body:there}", msg["content"])
	}
	if _, has := msg["template"]; has {
		t.Fatalf("template should be absent when content is used: %v", msg)
	}
	if want := `"requestId"`; !contains(stdout, want) {
		t.Fatalf("stdout = %q, want passthrough of %s", stdout, want)
	}
}

func TestSendEmailWithTemplateAndData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted, `{"requestId":"1-def"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv,
		"send", "--email", "a@b.com", "--template", "welcome",
		"--data", `{"name":"Ada"}`, "--brand-id", "brand_1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	msg := decodeBody(t, got.Body)["message"].(map[string]any)
	if msg["to"].(map[string]any)["email"] != "a@b.com" {
		t.Fatalf("to.email wrong: %v", msg["to"])
	}
	if msg["template"] != "welcome" {
		t.Fatalf("template = %v, want welcome", msg["template"])
	}
	if msg["brand_id"] != "brand_1" {
		t.Fatalf("brand_id = %v, want brand_1", msg["brand_id"])
	}
	data, ok := msg["data"].(map[string]any)
	if !ok || data["name"] != "Ada" {
		t.Fatalf("data = %v, want {name:Ada}", msg["data"])
	}
	if _, has := msg["content"]; has {
		t.Fatalf("content should be absent when template is used: %v", msg)
	}
}

func TestSendRoutingPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted, `{"requestId":"1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"send", "--list-id", "list_1", "--title", "x", "--body", "y",
		"--routing", `{"method":"single","channels":["email"]}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	msg := decodeBody(t, got.Body)["message"].(map[string]any)
	routing, ok := msg["routing"].(map[string]any)
	if !ok || routing["method"] != "single" {
		t.Fatalf("routing = %v, want method=single", msg["routing"])
	}
	if msg["to"].(map[string]any)["list_id"] != "list_1" {
		t.Fatalf("to.list_id wrong: %v", msg["to"])
	}
}

func TestSendAudienceRecipient(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted, `{"requestId":"1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "send", "--audience-id", "aud_1", "--template", "t")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	msg := decodeBody(t, got.Body)["message"].(map[string]any)
	if msg["to"].(map[string]any)["audience_id"] != "aud_1" {
		t.Fatalf("to.audience_id wrong: %v", msg["to"])
	}
}

func TestSendNoRecipientExits2(t *testing.T) {
	code, _, _ := run(t, nil, "send", "--title", "x", "--body", "y")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (no recipient)", code)
	}
}

func TestSendMultipleRecipientsExits2(t *testing.T) {
	code, _, _ := run(t, nil,
		"send", "--user-id", "u1", "--email", "a@b.com", "--title", "x", "--body", "y")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (multiple recipients)", code)
	}
}

func TestSendTemplateAndTitleExits2(t *testing.T) {
	code, _, _ := run(t, nil,
		"send", "--user-id", "u1", "--template", "t", "--title", "x", "--body", "y")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (template XOR content)", code)
	}
}

func TestSendNoContentExits2(t *testing.T) {
	code, _, _ := run(t, nil, "send", "--user-id", "u1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (no template or content)", code)
	}
}

func TestSendTitleWithoutBodyExits2(t *testing.T) {
	code, _, _ := run(t, nil, "send", "--user-id", "u1", "--title", "x")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (title needs body)", code)
	}
}

func TestSendInvalidDataJSONExits2(t *testing.T) {
	code, _, _ := run(t, nil,
		"send", "--user-id", "u1", "--template", "t", "--data", `{not json`)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (bad --data JSON)", code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
