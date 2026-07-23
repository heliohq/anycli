package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Reddit server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	UserAgent   string
	ContentType string
	Query       url.Values
	Form        url.Values
	Body        []byte
}

// newServer returns an httptest server answering every call with status +
// response, recording the last request into got.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return newServerWithHeaders(t, status, response, nil, got)
}

// newServerWithHeaders is newServer with extra response headers (rate limits).
func newServerWithHeaders(t *testing.T, status int, response string, headers map[string]string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			UserAgent:   r.Header.Get("User-Agent"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Form:        form,
			Body:        body,
		}
		for k, v := range headers {
			w.Header().Set(k, v)
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
	svc := &Service{APIBase: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "secret-reddit-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer secret-reddit-token" {
		t.Errorf("Authorization = %q, want Bearer secret-reddit-token", got.Auth)
	}
	if got.UserAgent != userAgent {
		t.Errorf("User-Agent = %q, want %q", got.UserAgent, userAgent)
	}
}

func assertRawJSON(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Query.Get("raw_json") != "1" {
		t.Errorf("raw_json = %q, want 1", got.Query.Get("raw_json"))
	}
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "REDDIT_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingToken_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "REDDIT_ACCESS_TOKEN is not set") {
		t.Errorf("envelope = %+v, want kind=usage with the missing-token message", env.Error)
	}
}

func TestExecute_UnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "frobnicate")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stderr == "" {
		t.Error("stderr is empty, want an unknown-command error")
	}
}

func TestMe_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"abc123","name":"helio_bot","link_karma":10,"comment_karma":5}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v1/me" {
		t.Errorf("request = %s %s, want GET /api/v1/me", got.Method, got.Path)
	}
	assertAuth(t, got)
	assertRawJSON(t, got)
	if !strings.Contains(stdout, "u/helio_bot") || !strings.Contains(stdout, "abc123") {
		t.Errorf("stdout = %q, want the terse identity line", stdout)
	}
}

func TestMe_JSONEmitsProviderObject(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"abc123","name":"helio_bot"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &me); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
	if me.ID != "abc123" || me.Name != "helio_bot" {
		t.Errorf("me = %+v, want id=abc123 name=helio_bot", me)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Unauthorized","error":401}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true on 401")
	}
	if !strings.Contains(stderr, "401") {
		t.Errorf("stderr = %q, want the HTTP status", stderr)
	}
}

func TestRateLimitSurfacesHeaders(t *testing.T) {
	var got capturedRequest
	srv := newServerWithHeaders(t, http.StatusTooManyRequests, `{"message":"Too Many Requests","error":429}`,
		map[string]string{"X-Ratelimit-Remaining": "0", "X-Ratelimit-Reset": "42"}, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false on 429")
	}
	if !strings.Contains(stderr, "remaining=0") || !strings.Contains(stderr, "reset=42") {
		t.Errorf("stderr = %q, want the rate-limit headers surfaced", stderr)
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"message":"Forbidden","error":403}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "me", "--json")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 403 {
		t.Errorf("envelope = %+v, want kind=api status=403", env.Error)
	}
}

func TestTokenIsRedactedFromErrors(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"bad secret-reddit-token here"}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "me")
	if strings.Contains(stderr, "secret-reddit-token") {
		t.Errorf("stderr leaks the token: %q", stderr)
	}
}
