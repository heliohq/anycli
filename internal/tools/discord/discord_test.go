package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// capturedRequest records what the fake Discord server saw.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   []byte
}

func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvBotToken: "bot-token-123"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"channels", "list", "--guild", "g1"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "DISCORD_BOT_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "missing access", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"message":"provider message","code":0}`, &got)
			defer srv.Close()

			result, _, _ := runResult(t, srv, "channels", "list", "--guild", "g1")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestMessageSend_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"msg-1","channel_id":"ch-1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "message", "send", "--channel", "ch-1", "--text", "hello discord")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/channels/ch-1/messages" {
		t.Errorf("request = %s %s, want POST /channels/ch-1/messages", got.Method, got.Path)
	}
	if got.Auth != "Bot bot-token-123" {
		t.Errorf("Authorization = %q, want Bot bot-token-123 (NOT Bearer)", got.Auth)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["content"] != "hello discord" {
		t.Errorf("content = %q, want hello discord", payload["content"])
	}
	if !strings.Contains(stdout, `"id":"msg-1"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestMessageSend_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"message":"Missing Access","code":50001}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "message", "send", "--channel", "ch-x", "--text", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Missing Access") || !strings.Contains(stderr, "50001") {
		t.Errorf("stderr = %q, want the Discord message and code", stderr)
	}
}

func TestChannelsList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":"ch-1","name":"general"}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "channels", "list", "--guild", "guild-1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/guilds/guild-1/channels" {
		t.Errorf("request = %s %s, want GET /guilds/guild-1/channels", got.Method, got.Path)
	}
	if got.Auth != "Bot bot-token-123" {
		t.Errorf("Authorization = %q, want Bot bot-token-123", got.Auth)
	}
	if !strings.Contains(stdout, `"general"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestChannelsList_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"message":"Unknown Guild","code":10004}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "channels", "list", "--guild", "missing")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Unknown Guild") {
		t.Errorf("stderr = %q, want the Discord message", stderr)
	}
}

// TestSideEffectAnnotations asserts every runnable leaf command of the tree
// carries an explicit anycli.side_effect annotation with the reviewed value
// (design 318 may-mutate criterion), and that group commands carry none.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"discord message send":  "true",  // POST /channels/{id}/messages
		"discord channels list": "false", // GET /guilds/{id}/channels
	}

	root := (&Service{}).NewCommandTree()
	got := map[string]string{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		val, ok := cmd.Annotations["anycli.side_effect"]
		if cmd.HasSubCommands() {
			if ok {
				t.Errorf("%s: group command must not carry anycli.side_effect, got %q", cmd.CommandPath(), val)
			}
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		if !ok {
			t.Errorf("%s: runnable leaf missing explicit anycli.side_effect annotation", cmd.CommandPath())
			return
		}
		got[cmd.CommandPath()] = val
	}
	walk(root)

	for path, wantVal := range want {
		if gotVal, ok := got[path]; !ok {
			t.Errorf("%s: leaf command not found in tree", path)
		} else if gotVal != wantVal {
			t.Errorf("%s: anycli.side_effect = %q, want %q", path, gotVal, wantVal)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s: new runnable leaf not covered by this table — classify it per the design 318 may-mutate criterion", path)
		}
	}
}
