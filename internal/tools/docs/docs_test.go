package docs

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

// recordedRequest is one request the fake Docs server saw.
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

// fixture is a fake Docs API server: routes keyed by "METHOD /v1/...", every
// request recorded in order, retry sleeps recorded instead of slept.
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
		BaseURL: f.srv.URL + "/v1",
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
	result, err := svc.Execute(context.Background(), []string{"documents", "get", "d1"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "DOCS_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"documents", "explode"}, "explode"},
		{"get without id", []string{"documents", "get"}, "accepts 1 arg"},
		{"bad format", []string{"documents", "get", "d1", "--format", "pdf"}, "--format must be"},
		{"bad suggestions", []string{"documents", "get", "d1", "--suggestions", "bogus"}, "--suggestions must be"},
		{"create without title", []string{"documents", "create"}, "--title is required"},
		{"append without body", []string{"documents", "append", "d1"}, "exactly one of --text or --body-file"},
		{"append both", []string{"documents", "append", "d1", "--text", "x", "--body-file", "f"}, "none of the others can be"},
		{"replace-all without find", []string{"documents", "replace-all", "d1", "--replace", "y"}, "--find is required"},
		{"batch-update without file", []string{"documents", "batch-update", "d1"}, "--requests-file is required"},
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

func TestExtractDocumentID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"1AbC_dEf-123", "1AbC_dEf-123", false},
		{"https://docs.google.com/document/d/1AbC_dEf-123/edit", "1AbC_dEf-123", false},
		{"https://docs.google.com/document/d/1AbC_dEf-123/edit?tab=t.0#heading=h.x", "1AbC_dEf-123", false},
		{"  1AbC_dEf-123  ", "1AbC_dEf-123", false},
		{"", "", true},
		{"https://docs.google.com/spreadsheets/d/xyz", "", true},
	}
	for _, tc := range cases {
		got, err := extractDocumentID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("extractDocumentID(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("extractDocumentID(%q) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
		}
	}
}

func TestErrorHints(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantHint     string
		wantRejected bool
	}{
		{"401 reconnect + reject", http.StatusUnauthorized, `{"error":{"status":"UNAUTHENTICATED","message":"invalid creds"}}`, "disconnect and reconnect", true},
		{"403 scope insufficient", http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"scope","details":[{"reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}]}}`, "disconnect and reconnect", false},
		{"403 permission denied", http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"the caller does not have permission"}}`, "shared with the connected account", false},
		{"404 not found", http.StatusNotFound, `{"error":{"status":"NOT_FOUND","message":"Requested entity was not found"}}`, "shared with the connected account", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1/documents/d1": {tc.status, tc.body},
			})
			result, _, stderr := f.run(t, "documents", "get", "d1")
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantHint) {
				t.Errorf("stderr = %q, want hint %q", stderr, tc.wantHint)
			}
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}
