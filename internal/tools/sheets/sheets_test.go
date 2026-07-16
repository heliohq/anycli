package sheets

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

// recordedRequest is one request the fake Sheets server saw.
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

// fixture is a fake Sheets API server: routes keyed by "METHOD /v4/...", every
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
		BaseURL: f.srv.URL + "/v4",
		HC:      f.srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep:   func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
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
	result, err := svc.Execute(context.Background(), []string{"spreadsheets", "get", "abc"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "SHEETS_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"spreadsheets", "explode"}, "explode"},
		{"values get without range", []string{"values", "get", "id1"}, "at least one --range"},
		{"values get bad render", []string{"values", "get", "id1", "--range", "A1", "--render", "pdf"}, "--render must be"},
		{"values update without range", []string{"values", "update", "id1", "--values-json", "[[1]]"}, "--range is required"},
		{"values update without values", []string{"values", "update", "id1", "--range", "A1"}, "provide --values-json or --csv-file"},
		{"values update both inputs", []string{"values", "update", "id1", "--range", "A1", "--values-json", "[[1]]", "--csv-file", "x.csv"}, "none of the others can be"},
		{"values clear without range", []string{"values", "clear", "id1"}, "at least one --range"},
		{"create without title", []string{"spreadsheets", "create"}, "--title is required"},
		{"batch-update without file", []string{"spreadsheets", "batch-update", "id1"}, "--request-file is required"},
		{"tabs add without title", []string{"tabs", "add", "id1"}, "--title is required"},
		{"tabs copy-to without dest", []string{"tabs", "copy-to", "id1", "--tab", "Sheet1"}, "--dest is required"},
		{"get without id", []string{"spreadsheets", "get"}, "accepts 1 arg"},
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
		"GET /v4/spreadsheets/id1": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "spreadsheets", "get", "id1")
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
		{"server failure", http.StatusInternalServerError, "INTERNAL", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// GET retries on 429/5xx; give the retries a route too.
			f := newFixture(t, map[string]route{
				"GET /v4/spreadsheets/id1": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "spreadsheets", "get", "id1")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestBearerTokenSent(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1": {http.StatusOK, `{"spreadsheetId":"id1","properties":{"title":"Budget"},"sheets":[]}`},
	})
	f.runOK(t, "spreadsheets", "get", "id1")
	got := f.last(t, "GET", "/v4/spreadsheets/id1")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
}
