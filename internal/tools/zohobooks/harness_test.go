package zohobooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one request the fake server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Zoho Books server: it answers each request from
// routes keyed by "METHOD /path" (the path is the full request path, including
// the /books/v3 prefix) and records every request into reqs. An unmatched
// route returns 404 with a Books-shaped integer-code error body.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		})
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			if s.status != http.StatusNoContent {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(s.status)
			if s.body != "" {
				_, _ = w.Write([]byte(s.body))
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":1001,"message":"not found"}`))
	}))
}

// runResult bundles one Execute invocation's outcome.
type runResult struct {
	result execution.Result
	stdout string
	stderr string
}

// run executes the service against the fake server with the given args and a
// valid seeded token, capturing stdout/stderr.
func run(t *testing.T, server *httptest.Server, args ...string) runResult {
	t.Helper()
	return runWithEnv(t, server, map[string]string{EnvToken: "test-token"}, args...)
}

// runWithEnv is run with an explicit credential env map (to exercise the
// missing-token path).
func runWithEnv(t *testing.T, server *httptest.Server, env map[string]string, args ...string) runResult {
	t.Helper()
	var out, errBuf bytes.Buffer
	base := ""
	if server != nil {
		base = server.URL
	}
	svc := &Service{BaseURL: base, HC: server.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return runResult{result: res, stdout: out.String(), stderr: errBuf.String()}
}

// findReq returns the first recorded request matching method+path, or nil.
func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

// bodyMap decodes a request body into a generic JSON object.
func bodyMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, b)
	}
	return m
}
