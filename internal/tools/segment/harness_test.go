package segment

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

// capturedRequest records one request the fake Segment server received.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Accept string
	Query  url.Values
	Body   []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Segment Public API server: it answers each
// request from routes keyed by "METHOD /path" and records every request into
// reqs. An unmatched route returns 404 so callers can assert not-found handling.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"),
			Query:  r.URL.Query(),
			Body:   body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"type":"not_found","message":"resource not found"}]}`))
	}))
}

// runResult drives one segment invocation against srv and returns the outcome
// plus captured stdout / stderr.
type runResult struct {
	result execution.Result
	stdout string
	stderr string
}

// run executes the segment service with args against the fake server, injecting
// the given token via env (empty token means "do not set SEGMENT_TOKEN").
func run(t *testing.T, srv *httptest.Server, token string, args ...string) runResult {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.BaseURL = srv.URL
	}
	env := map[string]string{}
	if token != "" {
		env[EnvToken] = token
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil, outcome is in Result): %v", err)
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
