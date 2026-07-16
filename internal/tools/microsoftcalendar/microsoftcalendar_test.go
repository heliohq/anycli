package microsoftcalendar

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
	Body   []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake Graph API server: routes keyed by "METHOD /v1.0/...", every
// request recorded in order. Retry backoff sleeps are recorded instead of slept
// so tests stay fast and deterministic.
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
	result, err := svc.Execute(context.Background(), []string{"calendars", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MICROSOFT_CALENDAR_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"events", "explode"}, "explode"},
		{"unknown group", []string{"nope"}, "nope"},
		{"get without id", []string{"events", "get"}, "accepts 1 arg"},
		{"create without required", []string{"events", "create", "--subject", "x"}, "required flag"},
		{"update without id", []string{"events", "update"}, "accepts 1 arg"},
		{"respond bad action", []string{"events", "respond", "e1", "--action", "maybe"}, "--action must be"},
		{"respond without action", []string{"events", "respond", "e1"}, "required flag"},
		{"freebusy without window", []string{"freebusy"}, "required flag"},
		{"events list only-start", []string{"events", "list", "--start", "2026-01-01T00:00:00Z"}, "must be given together"},
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
		"GET /v1.0/me/calendars": {http.StatusForbidden, `{"error":{"code":"ErrorAccessDenied","message":"insufficient privileges"}}`},
	})
	result, _, stderr := f.run(t, "calendars", "list")
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
		t.Error("403 must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		code         string
		wantRejected bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "SomethingElse", true},
		{"invalid auth token code", http.StatusBadRequest, "InvalidAuthenticationToken", true},
		{"forbidden missing scope", http.StatusForbidden, "ErrorAccessDenied", false},
		{"rate limited", http.StatusTooManyRequests, "TooManyRequests", false},
		{"server failure", http.StatusInternalServerError, "InternalServerError", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1.0/me/calendars": {tc.status, `{"error":{"code":"` + tc.code + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "calendars", "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestScopeHintAbsentOnPlainError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/calendars": {http.StatusBadRequest, `{"error":{"code":"BadRequest","message":"bad request"}}`},
	})
	_, _, stderr := f.run(t, "calendars", "list")
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}
