package microsoftoutlook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// recordedRequest is one request the fake Graph server saw.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Prefer string
	Body   []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake Microsoft Graph API server: routes keyed by
// "METHOD /v1.0/...", every request recorded in order. Retry backoff sleeps are
// recorded instead of slept so tests stay fast and deterministic.
type fixture struct {
	srv      *httptest.Server
	requests []recordedRequest
	sleeps   []time.Duration
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Prefer: r.Header.Get("Prefer"),
			Body:   body.Bytes(),
		})
		rt, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"NotFound","message":"no route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// last returns the most recent request matching method+path.
func (f *fixture) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	t.Fatalf("no recorded request %s %s", method, path)
	return recordedRequest{}
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL: f.srv.URL + "/v1.0",
		HC:      f.srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep:   func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func (f *fixture) runOK(t *testing.T, args ...string) string {
	t.Helper()
	result, stdout, stderr := f.run(t, args...)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	return stdout
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"messages", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MICROSOFT_OUTLOOK_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"messages", "explode"}, "explode"},
		{"invalid body kind", []string{"messages", "get", "m1", "--body", "pdf"}, "--body must be text or html"},
		{"send without body", []string{"messages", "send", "--to", "a@b.c", "--subject", "x"}, "at least one of the flags"},
		{"send body and body-file", []string{"messages", "send", "--to", "a@b.c", "--subject", "x", "--body", "y", "--body-file", "z"}, "none of the others can be"},
		{"send without to", []string{"messages", "send", "--subject", "x", "--body", "y"}, `"to" not set`},
		{"mark without action", []string{"messages", "mark", "m1"}, "nothing to mark"},
		{"mark read and unread", []string{"messages", "mark", "m1", "--read", "--unread"}, "none of the others can be"},
		{"move without folder", []string{"messages", "move", "m1"}, "--folder is required"},
		{"get without id", []string{"messages", "get"}, "accepts 1 arg"},
		{"forward without to", []string{"messages", "forward", "m1"}, `"to" not set`},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestScopeHintOn403(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/mailFolders": {http.StatusForbidden, `{"error":{"code":"ErrorAccessDenied","message":"insufficient privileges"}}`},
	})
	result, _, stderr := f.run(t, "folders", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient privileges") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 access denied must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		providerCode string
		wantRejected bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "SomeCode", true},
		{"invalid auth token", http.StatusBadRequest, "InvalidAuthenticationToken", true},
		{"access denied", http.StatusForbidden, "ErrorAccessDenied", false},
		{"rate limited", http.StatusTooManyRequests, "TooManyRequests", false},
		{"server failure", http.StatusInternalServerError, "InternalServerError", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1.0/me/mailFolders": {tc.status, `{"error":{"code":"` + tc.providerCode + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "folders", "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestScopeHintAbsentOnPlainError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/mailFolders": {http.StatusBadRequest, `{"error":{"code":"BadRequest","message":"bad request"}}`},
	})
	_, _, stderr := f.run(t, "folders", "list")
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}
