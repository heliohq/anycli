package jotform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Jotform server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Query       string
	Accept      string
	APIKey      string
	ContentType string
	Body        []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Accept:      r.Header.Get("Accept"),
			APIKey:      r.Header.Get(authHeader),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

const testKey = "key-abc-123"

// run executes one jotform invocation against the fake server and returns exit
// code + captured streams. The API key is delivered exactly as the token
// gateway would deliver it: through the injected env var.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: testKey})
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

// parseForm parses a captured x-www-form-urlencoded body into url.Values.
func parseForm(t *testing.T, raw []byte) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatalf("bad form body %q: %v", raw, err)
	}
	return v
}
