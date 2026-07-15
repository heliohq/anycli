package gmail

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodeRaw extracts and decodes the raw MIME message from a send/draft
// request body, returning the parsed payload map too.
func decodeRaw(t *testing.T, body []byte) (mime string, payload map[string]any) {
	t.Helper()
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	rawAny := payload["raw"]
	if rawAny == nil {
		if msg, ok := payload["message"].(map[string]any); ok {
			rawAny = msg["raw"]
		}
	}
	raw, ok := rawAny.(string)
	if !ok || raw == "" {
		t.Fatalf("payload %v carries no raw field", payload)
	}
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("raw is not base64url: %v", err)
	}
	return string(decoded), payload
}

func TestSend_PlainBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m9","threadId":"t9"}`},
	})
	stdout := f.runOK(t, "messages", "send",
		"--to", "a@b.c,d@e.f", "--cc", "g@h.i", "--bcc", "j@k.l",
		"--subject", "Hi there", "--body", "hello body")
	got := f.last(t, "POST", "/gmail/v1/users/me/messages/send")
	mime, payload := decodeRaw(t, got.Body)
	for _, want := range []string{
		"To: a@b.c, d@e.f\r\n",
		"Cc: g@h.i\r\n",
		"Bcc: j@k.l\r\n",
		"Subject: Hi there\r\n",
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n",
		"hello body",
	} {
		if !strings.Contains(mime, want) {
			t.Errorf("MIME = %q, want it to contain %q", mime, want)
		}
	}
	if _, ok := payload["threadId"]; ok {
		t.Error("plain send must not set threadId")
	}
	if !strings.Contains(stdout, "sent message m9") {
		t.Errorf("human output = %q, want the sent summary", stdout)
	}
}

func TestSend_HTMLBodyFromFile(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(bodyFile, []byte("<p>from file</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m9","threadId":"t9"}`},
	})
	f.runOK(t, "messages", "send", "--to", "a@b.c", "--subject", "x", "--body-file", bodyFile, "--html")
	mime, _ := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	if !strings.Contains(mime, "Content-Type: text/html; charset=\"UTF-8\"\r\n") {
		t.Errorf("MIME = %q, want the text/html content type", mime)
	}
	if !strings.Contains(mime, "<p>from file</p>") {
		t.Errorf("MIME = %q, want the body-file content", mime)
	}
}

func TestSend_MultipartWithAttachment(t *testing.T) {
	attach := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(attach, []byte("attachment bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m9","threadId":"t9"}`},
	})
	f.runOK(t, "messages", "send", "--to", "a@b.c", "--subject", "x", "--body", "see attached", "--attach", attach)
	mime, _ := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	for _, want := range []string{
		"Content-Type: multipart/mixed; boundary=",
		"see attached",
		`Content-Disposition: attachment; filename="notes.txt"`,
		"Content-Transfer-Encoding: base64",
		base64.StdEncoding.EncodeToString([]byte("attachment bytes")),
	} {
		if !strings.Contains(mime, want) {
			t.Errorf("MIME = %q, want it to contain %q", mime, want)
		}
	}
}

func TestBuildMIME_RejectsOversizeMessage(t *testing.T) {
	_, err := buildMIME(mimeMessage{
		to:      []string{"a@b.c"},
		subject: "big",
		body:    strings.Repeat("a", maxMessageBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "25MB") {
		t.Errorf("err = %v, want the 25MB limit error", err)
	}
}

func TestReply_BuildsThreadHeaders(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":    {http.StatusOK, fullMessage("m1")},
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m10","threadId":"t9"}`},
	})
	f.runOK(t, "messages", "reply", "m1", "--body", "thanks!")
	mime, payload := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	if payload["threadId"] != "t9" {
		t.Errorf("threadId = %v, want the original t9", payload["threadId"])
	}
	for _, want := range []string{
		"To: Ada Lovelace <ada@example.com>\r\n",
		"Subject: Re: Quarterly numbers\r\n",
		"In-Reply-To: <orig-123@mail.example.com>\r\n",
		"References: <root-1@mail.example.com> <orig-123@mail.example.com>\r\n",
		"thanks!",
	} {
		if !strings.Contains(mime, want) {
			t.Errorf("MIME = %q, want it to contain %q", mime, want)
		}
	}
	if strings.Contains(mime, "Cc:") {
		t.Errorf("MIME = %q, sender-only reply must not carry Cc", mime)
	}
}

func TestReply_AllExcludesSelfAndSender(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":    {http.StatusOK, fullMessage("m1")},
		"GET /gmail/v1/users/me/profile":        {http.StatusOK, `{"emailAddress":"me@example.com"}`},
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m10","threadId":"t9"}`},
	})
	f.runOK(t, "messages", "reply", "m1", "--all", "--body", "reply all")
	mime, _ := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	if !strings.Contains(mime, "To: Ada Lovelace <ada@example.com>\r\n") {
		t.Errorf("MIME = %q, want To = the original sender", mime)
	}
	ccLine := ""
	for _, line := range strings.Split(mime, "\r\n") {
		if strings.HasPrefix(line, "Cc: ") {
			ccLine = line
		}
	}
	if !strings.Contains(ccLine, "bob@example.com") || !strings.Contains(ccLine, "carol@example.com") {
		t.Errorf("Cc line = %q, want the other To + Cc recipients", ccLine)
	}
	if strings.Contains(ccLine, "me@example.com") {
		t.Errorf("Cc line = %q, must exclude the connected mailbox", ccLine)
	}
	if strings.Contains(ccLine, "ada@example.com") {
		t.Errorf("Cc line = %q, must exclude the reply target", ccLine)
	}
}

func TestReply_SubjectAlreadyRe(t *testing.T) {
	msg := strings.Replace(fullMessage("m1"), "Quarterly numbers", "Re: Quarterly numbers", 1)
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":    {http.StatusOK, msg},
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m10","threadId":"t9"}`},
	})
	f.runOK(t, "messages", "reply", "m1", "--body", "x")
	mime, _ := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	if strings.Contains(mime, "Re: Re:") {
		t.Errorf("MIME = %q, must not double the Re: prefix", mime)
	}
}

func TestForward_QuotesOriginal(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":    {http.StatusOK, fullMessage("m1")},
		"POST /gmail/v1/users/me/messages/send": {http.StatusOK, `{"id":"m11","threadId":"t11"}`},
	})
	f.runOK(t, "messages", "forward", "m1", "--to", "new@example.com", "--body", "FYI")
	mime, payload := decodeRaw(t, f.last(t, "POST", "/gmail/v1/users/me/messages/send").Body)
	for _, want := range []string{
		"To: new@example.com\r\n",
		"Subject: Fwd: Quarterly numbers\r\n",
		"FYI\n\n---------- Forwarded message ---------\n",
		"From: Ada Lovelace <ada@example.com>\n",
		"Subject: Quarterly numbers\n",
		"plain body!",
	} {
		if !strings.Contains(mime, want) {
			t.Errorf("MIME = %q, want it to contain %q", mime, want)
		}
	}
	if _, ok := payload["threadId"]; ok {
		t.Error("forward must not set threadId")
	}
	if strings.Contains(mime, "In-Reply-To:") {
		t.Errorf("MIME = %q, forward must not carry reply headers", mime)
	}
}
