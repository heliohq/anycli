package calendar

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

// recordedRequest is one request the fake Calendar server saw.
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

// fixture is a fake Calendar API server: routes keyed by "METHOD /calendar/v3/
// ...", every request recorded in order. Retry backoff sleeps are recorded
// instead of slept so tests stay fast and deterministic.
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
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
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
		BaseURL:      f.srv.URL + "/calendar/v3",
		HC:           f.srv.Client(),
		Out:          &out,
		Err:          &errBuf,
		sleep:        func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
		newRequestID: func() string { return "req-fixed" },
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
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
	if !strings.Contains(errBuf.String(), "CALENDAR_ACCESS_TOKEN is not set") {
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
		{"create without summary", []string{"events", "create", "--from", "2026-07-16T10:00:00-07:00", "--to", "2026-07-16T11:00:00-07:00"}, "--summary is required"},
		{"create without times", []string{"events", "create", "--summary", "x"}, "--from and --to are required"},
		{"create bad rfc3339", []string{"events", "create", "--summary", "x", "--from", "tomorrow", "--to", "2026-07-16T11:00:00-07:00"}, "must be an RFC3339"},
		{"create bad send-updates", []string{"events", "create", "--summary", "x", "--from", "2026-07-16T10:00:00-07:00", "--to", "2026-07-16T11:00:00-07:00", "--send-updates", "maybe"}, "--send-updates must be"},
		{"update nothing", []string{"events", "update", "e1"}, "nothing to update"},
		{"respond bad status", []string{"events", "respond", "e1", "--status", "meh"}, "--status must be"},
		{"list bad order-by", []string{"events", "list", "--order-by", "priority"}, "--order-by must be"},
		{"list startTime without single-events", []string{"events", "list", "--order-by", "startTime"}, "requires --single-events"},
		{"freebusy without calendar", []string{"freebusy", "--from", "2026-07-16T10:00:00-07:00", "--to", "2026-07-16T11:00:00-07:00"}, "--calendar is required"},
		{"get without id", []string{"events", "get"}, "accepts 1 arg"},
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
		"GET /calendar/v3/users/me/calendarList": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "calendars", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "UNKNOWN", true},
		{"explicit unauthenticated status", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"rate limited", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes := map[string]route{
				"GET /calendar/v3/users/me/calendarList/primary": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			}
			f := newFixture(t, routes)
			result, _, _ := f.run(t, "calendars", "get", "primary")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestScopeHintAbsentOnPlainError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/users/me/calendarList": {http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT","message":"bad request"}}`},
	})
	_, _, stderr := f.run(t, "calendars", "list")
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}
