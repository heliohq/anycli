package kit

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

// capturedRequest records one request the fake Kit server received.
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

// newMux is a multi-route fake Kit server: it answers each request from routes
// keyed by "METHOD /path" and records every request into reqs. An unmatched
// route returns 404 so unexpected calls surface as failures.
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
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["not found"]}`))
	}))
}

// run executes one kit invocation against the fake server, returning captured
// stdout, stderr, and the execution result.
func run(t *testing.T, base string, env map[string]string, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: base, Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a non-nil error (should be nil, exit code carries failure): %v", err)
	}
	return out.String(), errBuf.String(), res
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

// decode unmarshals a JSON blob into a generic map, failing the test on error.
func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not a JSON object: %v (%s)", err, b)
	}
	return m
}

// tokenEnv is the standard resolved-credential env for tests.
func tokenEnv() map[string]string { return map[string]string{EnvToken: "T"} }
