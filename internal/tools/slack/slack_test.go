package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records what the fake Slack server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// newServer returns an httptest server answering every call with response,
// recording the last request into got.
func newServer(t *testing.T, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Slack errors still come back HTTP 200
		_, _ = w.Write([]byte(response))
	}))
}

// run executes the service against srv with a bot token, capturing output.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	code, err := svc.Execute(context.Background(), args, map[string]string{EnvBotToken: "xoxb-test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return code, out.String(), errBuf.String()
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	code, err := svc.Execute(context.Background(), []string{"channels", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "SLACK_BOT_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestChatPost_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":true,"channel":"C123","ts":"1.2"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "chat", "post", "--channel", "C123", "--text", "hello world")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/chat.postMessage" {
		t.Errorf("request = %s %s, want POST /chat.postMessage", got.Method, got.Path)
	}
	if got.Auth != "Bearer xoxb-test-token" {
		t.Errorf("Authorization = %q, want Bearer xoxb-test-token", got.Auth)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["channel"] != "C123" || payload["text"] != "hello world" {
		t.Errorf("payload = %v, want channel/text set", payload)
	}
	if !strings.Contains(stdout, `"ok":true`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestChatPost_OKFalse(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":false,"error":"channel_not_found"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "chat", "post", "--channel", "CBAD", "--text", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (HTTP 200 + ok:false is a failure)", code)
	}
	if !strings.Contains(stderr, "channel_not_found") {
		t.Errorf("stderr = %q, want the Slack error code", stderr)
	}
}

func TestChatHistory_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":true,"messages":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "chat", "history", "--channel", "C123", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/conversations.history" {
		t.Errorf("request = %s %s, want GET /conversations.history", got.Method, got.Path)
	}
	if got.Auth != "Bearer xoxb-test-token" {
		t.Errorf("Authorization = %q, want Bearer xoxb-test-token", got.Auth)
	}
	if !strings.Contains(got.Query, "channel=C123") || !strings.Contains(got.Query, "limit=5") {
		t.Errorf("query = %q, want channel + limit", got.Query)
	}
	if !strings.Contains(stdout, `"messages"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestChatHistory_OKFalse(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":false,"error":"not_in_channel"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "chat", "history", "--channel", "C123")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "not_in_channel") {
		t.Errorf("stderr = %q, want the Slack error code", stderr)
	}
}

func TestChannelsList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":true,"channels":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "channels", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/conversations.list" {
		t.Errorf("request = %s %s, want GET /conversations.list", got.Method, got.Path)
	}
	if got.Auth != "Bearer xoxb-test-token" {
		t.Errorf("Authorization = %q, want Bearer xoxb-test-token", got.Auth)
	}
	if !strings.Contains(got.Query, "types=") {
		t.Errorf("query = %q, want channel types filter", got.Query)
	}
	if !strings.Contains(stdout, `"channels"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestChannelsList_OKFalse(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":false,"error":"invalid_auth"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "channels", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid_auth") {
		t.Errorf("stderr = %q, want the Slack error code", stderr)
	}
}

func TestChatPost_MissingRequiredFlag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"ok":true}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "chat", "post", "--text", "no channel")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for a missing required flag", code)
	}
	if stderr == "" {
		t.Error("expected a flag error on stderr")
	}
	if got.Path != "" {
		t.Errorf("no request must be sent on flag errors, got %s", got.Path)
	}
}
