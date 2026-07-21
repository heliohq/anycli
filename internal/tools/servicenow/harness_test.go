package servicenow

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

// capturedRequest records what the fake ServiceNow server saw.
type capturedRequest struct {
	Method      string
	Path        string
	APIKey      string
	Accept      string
	ContentType string
	Query       url.Values
	Header      http.Header
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake ServiceNow Table API server: it answers each
// request from routes keyed by "METHOD /path" and records every request into
// reqs. An unmatched route returns a ServiceNow-shaped 404 error envelope.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			APIKey:      r.Header.Get(apiKeyHeader),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Header:      r.Header.Clone(),
			Body:        body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = io.WriteString(w, s.body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"No Record found","detail":"not found"},"status":"failure"}`)
	}))
}

// newServer returns a single-route httptest server answering every call with
// status + response, recording the last request into got.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			APIKey:      r.Header.Get(apiKeyHeader),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Header:      r.Header.Clone(),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
}

const testAPIKey = "secret-sn-key"

// run executes the servicenow service against srv, deriving the instance base
// URL from the SERVICENOW_INSTANCE_URL env (exercising the base-URL-from-
// credential path) and injecting the api key. Returns exit code + streams.
func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{EnvInstanceURL: srv.URL, EnvAPIKey: testAPIKey}
	result, err := svc.Execute(context.Background(), args, env)
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
