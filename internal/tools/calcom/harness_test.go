package calcom

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

// capturedRequest records what the fake Cal.com server saw.
type capturedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Auth    string
	Version string
	Accept  string
	Body    []byte
}

// route keys a canned reply by "METHOD /path".
type route struct {
	status int
	body   string
}

// newServer is a multi-route fake Cal.com v2 server: it answers each request
// from routes keyed by "METHOD /path" and records every request. An unmatched
// route returns 404 with a v2 error envelope.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]route) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.Query(),
			Auth:    r.Header.Get("Authorization"),
			Version: r.Header.Get("cal-api-version"),
			Accept:  r.Header.Get("Accept"),
			Body:    body,
		})
		w.Header().Set("Content-Type", "application/json")
		if rt, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(rt.status)
			_, _ = w.Write([]byte(rt.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"error","error":{"code":"NOT_FOUND","message":"not found"}}`))
	}))
}

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

// runResult is run but returns the full Result (for CredentialRejected assertions).
func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
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
