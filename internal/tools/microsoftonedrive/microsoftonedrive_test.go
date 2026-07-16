package microsoftonedrive

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
	Method      string
	Path        string
	Query       string
	Auth        string
	ContentType string
	ContentRng  string
	Body        []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake Microsoft Graph API server: routes keyed by
// "METHOD /v1.0/...", every request recorded in order. Retry backoff sleeps
// are recorded instead of slept so tests stay fast and deterministic.
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
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			ContentRng:  r.Header.Get("Content-Range"),
			Body:        body.Bytes(),
		})
		rt, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"itemNotFound","message":"no route"}}`))
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
	result, err := svc.Execute(context.Background(), []string{"items", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MICROSOFT_ONEDRIVE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"items", "explode"}, "explode"},
		{"unknown top-level", []string{"teleport"}, "teleport"},
		{"get id and path", []string{"items", "get", "id1", "--path", "a/b"}, "not both"},
		{"get without target", []string{"items", "get"}, "id or --path is required"},
		{"mkdir without name", []string{"items", "mkdir"}, `"name" not set`},
		{"move without to", []string{"items", "move", "id1"}, `"to" not set`},
		{"rename without name", []string{"items", "rename", "id1"}, `"name" not set`},
		{"share bad type", []string{"items", "share", "id1", "--type", "own"}, "--type must be view or edit"},
		{"share bad scope", []string{"items", "share", "id1", "--scope", "world"}, "--scope must be anonymous or organization"},
		{"search without query", []string{"search"}, `"query" not set`},
		{"list path and parent", []string{"items", "list", "--path", "a", "--parent", "b"}, "none of the others can be"},
		{"delete without id", []string{"items", "delete"}, "requires at least 1 arg"},
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
		"GET /v1.0/me/drive/root/children": {http.StatusForbidden, `{"error":{"code":"accessDenied","message":"insufficient scope"}}`},
	})
	result, _, stderr := f.run(t, "items", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient scope") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 accessDenied must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		code         string
		wantRejected bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "unknown", true},
		{"invalid auth token code", http.StatusBadRequest, "InvalidAuthenticationToken", true},
		{"access denied", http.StatusForbidden, "accessDenied", false},
		{"rate limited", http.StatusTooManyRequests, "activityLimitReached", false},
		{"server failure", http.StatusInternalServerError, "generalException", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1.0/me/drive/items/id1": {tc.status, `{"error":{"code":"` + tc.code + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "items", "get", "id1")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestScopeHintAbsentOnPlainError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/items/id1": {http.StatusBadRequest, `{"error":{"code":"invalidRequest","message":"bad request"}}`},
	})
	_, _, stderr := f.run(t, "items", "get", "id1")
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}

func TestRetryOn5xxThenSuccess(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"serviceNotAvailable","message":"try again"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"id1","name":"f.txt","size":3}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	var sleeps []time.Duration
	svc := &Service{
		BaseURL: srv.URL + "/v1.0",
		HC:      srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep:   func(d time.Duration) { sleeps = append(sleeps, d) },
	}
	result, err := svc.Execute(context.Background(), []string{"items", "get", "id1"}, map[string]string{EnvAccessToken: "test-token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %s)", result.ExitCode, errBuf.String())
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2 (one retry)", calls)
	}
	if len(sleeps) != 1 {
		t.Errorf("recorded sleeps = %d, want 1 backoff", len(sleeps))
	}
}
