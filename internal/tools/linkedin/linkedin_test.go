package linkedin

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
)

// capturedRequest records what the fake LinkedIn server saw.
type capturedRequest struct {
	Method   string
	Path     string
	Auth     string
	Version  string
	Protocol string
	Body     []byte
}

func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			Auth:     r.Header.Get("Authorization"),
			Version:  r.Header.Get("LinkedIn-Version"),
			Protocol: r.Header.Get("X-Restli-Protocol-Version"),
			Body:     body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func run(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, env, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{APIBase: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func fullEnv() map[string]string {
	return map[string]string{
		EnvAccessToken: "li-token",
		EnvPersonURN:   "urn:li:person:abc123",
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
	if !strings.Contains(errBuf.String(), "LINKEDIN_ACCESS_TOKEN is not set") {
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
		{name: "missing permission", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "token verification outage", status: http.StatusInternalServerError, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"message":"provider message","serviceErrorCode":0}`, &got)
			defer srv.Close()

			result, _, _ := runResult(t, srv, fullEnv(), "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestPostCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"urn:li:share:1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(), "post", "create", "--text", "Hello LinkedIn")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/rest/posts" {
		t.Errorf("request = %s %s, want POST /rest/posts", got.Method, got.Path)
	}
	if got.Auth != "Bearer li-token" {
		t.Errorf("Authorization = %q, want Bearer li-token", got.Auth)
	}
	if got.Version != "202406" {
		t.Errorf("LinkedIn-Version = %q, want 202406", got.Version)
	}
	if got.Protocol != "2.0.0" {
		t.Errorf("X-Restli-Protocol-Version = %q, want 2.0.0", got.Protocol)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["author"] != "urn:li:person:abc123" {
		t.Errorf("author = %v, want the injected person URN", payload["author"])
	}
	if payload["commentary"] != "Hello LinkedIn" {
		t.Errorf("commentary = %v, want the post text", payload["commentary"])
	}
	if payload["lifecycleState"] != "PUBLISHED" {
		t.Errorf("lifecycleState = %v, want PUBLISHED", payload["lifecycleState"])
	}
	if !strings.Contains(stdout, "urn:li:share:1") {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestPostCreate_MissingPersonURN(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	env := map[string]string{EnvAccessToken: "li-token"} // person_urn absent → injected empty
	code, _, stderr := run(t, srv, env, "post", "create", "--text", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 when person_urn is missing", code)
	}
	if !strings.Contains(stderr, "person_urn missing — reconnect LinkedIn to capture it") {
		t.Errorf("stderr = %q, want the reconnect hint", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent without a person URN, got %s", got.Path)
	}
}

func TestPostCreate_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"message":"Not enough permissions to access: posts.CREATE","serviceErrorCode":100}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "post", "create", "--text", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Not enough permissions") {
		t.Errorf("stderr = %q, want the LinkedIn message", stderr)
	}
}

func TestMe_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"sub":"abc123","name":"Test User"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(), "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/userinfo" {
		t.Errorf("request = %s %s, want GET /v2/userinfo", got.Method, got.Path)
	}
	if got.Auth != "Bearer li-token" {
		t.Errorf("Authorization = %q, want Bearer li-token", got.Auth)
	}
	if got.Version != "" {
		t.Errorf("LinkedIn-Version = %q on /v2/userinfo, want unset (unversioned API)", got.Version)
	}
	if !strings.Contains(stdout, `"sub":"abc123"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestMe_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Invalid access token","serviceErrorCode":65600}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "me")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Invalid access token") {
		t.Errorf("stderr = %q, want the LinkedIn message", stderr)
	}
}
