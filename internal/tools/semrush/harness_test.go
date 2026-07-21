package semrush

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Semrush server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
}

// newServer returns a single-route server that records the request and replies
// with the given status/plain-text body for every path. Semrush responses are
// text (semicolon CSV or an ERROR line), not JSON.
func newServer(t *testing.T, status int, body string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = capturedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// run drives one semrush invocation against a fake server (reports base) with a
// seeded API key, returning the exit code and captured streams.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{ReportsBaseURL: srv.URL, UnitsBaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: "key-abcd1234"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// parseQuery parses a raw query string into url.Values.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

// decodeEnvelope unmarshals stdout into the report JSON envelope.
func decodeEnvelope(t *testing.T, stdout string) reportEnvelope {
	t.Helper()
	var env reportEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a report envelope: %v (%s)", err, stdout)
	}
	return env
}
