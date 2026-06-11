package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records what the fake Google server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// newServer returns an httptest server answering every call with status +
// response, recording the last request into got.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
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
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes the service with all three bases pointed at srv.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		GmailBase: srv.URL + "/gmail/v1",
		CalBase:   srv.URL + "/calendar/v3",
		DriveBase: srv.URL + "/drive/v3",
		HC:        srv.Client(),
		Out:       &out,
		Err:       &errBuf,
	}
	code, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return code, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want Bearer ya29.test-token", got.Auth)
	}
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	code, err := svc.Execute(context.Background(), []string{"drive", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "GOOGLE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestGmailSend_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"m1","labelIds":["SENT"]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "gmail", "send", "--to", "a@b.c", "--subject", "Hi", "--body", "hello there")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/gmail/v1/users/me/messages/send" {
		t.Errorf("request = %s %s, want POST .../users/me/messages/send", got.Method, got.Path)
	}
	assertAuth(t, got)
	var payload map[string]string
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	raw, err := base64.URLEncoding.DecodeString(payload["raw"])
	if err != nil {
		t.Fatalf("raw is not base64url: %v", err)
	}
	msg := string(raw)
	if !strings.Contains(msg, "To: a@b.c") || !strings.Contains(msg, "Subject: Hi") || !strings.Contains(msg, "hello there") {
		t.Errorf("RFC822 message = %q, want To/Subject/body", msg)
	}
	if !strings.Contains(stdout, `"id":"m1"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestGmailSend_ForbiddenAppendsScopeHint(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "gmail", "send", "--to", "a@b.c", "--subject", "x", "--body", "y")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
}

func TestGmailList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"messages":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "gmail", "list", "--query", "is:unread", "--limit", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/gmail/v1/users/me/messages" {
		t.Errorf("request = %s %s, want GET .../users/me/messages", got.Method, got.Path)
	}
	assertAuth(t, got)
	if !strings.Contains(got.Query, "q=is%3Aunread") || !strings.Contains(got.Query, "maxResults=3") {
		t.Errorf("query = %q, want q + maxResults", got.Query)
	}
}

func TestGmailList_UnauthorizedAppendsScopeHint(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":{"status":"UNAUTHENTICATED","message":"invalid credentials"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "gmail", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 401", stderr)
	}
}

func TestCalendarList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "calendar", "list", "--days", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/calendar/v3/calendars/primary/events" {
		t.Errorf("request = %s %s, want GET .../calendars/primary/events", got.Method, got.Path)
	}
	assertAuth(t, got)
	for _, param := range []string{"timeMin=", "timeMax=", "singleEvents=true", "orderBy=startTime"} {
		if !strings.Contains(got.Query, param) {
			t.Errorf("query = %q, want %q", got.Query, param)
		}
	}
}

func TestCalendarCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1","status":"confirmed"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "calendar", "create",
		"--title", "Standup",
		"--start", "2026-06-15T10:00:00Z",
		"--end", "2026-06-15T10:30:00Z",
		"--attendees", "a@b.c, d@e.f")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/calendar/v3/calendars/primary/events" {
		t.Errorf("request = %s %s, want POST .../calendars/primary/events", got.Method, got.Path)
	}
	assertAuth(t, got)
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["summary"] != "Standup" {
		t.Errorf("summary = %v, want Standup", payload["summary"])
	}
	attendees, _ := payload["attendees"].([]any)
	if len(attendees) != 2 {
		t.Errorf("attendees = %v, want 2 entries", payload["attendees"])
	}
}

func TestCalendarCreate_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT","message":"bad time range"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "calendar", "create", "--title", "x", "--start", "bad", "--end", "bad")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "bad time range") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}

func TestDriveList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"files":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "drive", "list", "--query", "name contains 'spec'")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/drive/v3/files" {
		t.Errorf("request = %s %s, want GET .../files", got.Method, got.Path)
	}
	assertAuth(t, got)
	if !strings.Contains(got.Query, "q=") {
		t.Errorf("query = %q, want the q param", got.Query)
	}
	if !strings.Contains(stdout, `"files"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestDriveList_ForbiddenAppendsScopeHint(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"drive scope missing"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "drive", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
}
