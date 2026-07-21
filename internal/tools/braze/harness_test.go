package braze

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

// testKey is the REST API key embedded in the DSN every harness run passes
// through the credential env var. Tests assert it is reconstructed into the
// Authorization: Bearer header and never logged.
const testKey = "a1b2c3d4-key"

// testDSN is a well-formed US-cluster credential (key in userinfo, cluster in
// host). The HTTP base is overridden to the httptest server, but the apiKey is
// still taken from this DSN's userinfo — proving Bearer assembly from the DSN.
const testDSN = "https://" + testKey + "@rest.iad-05.braze.com"

// capturedRequest records what the fake Braze server saw for one request.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route, plus optional headers
// (used to set the X-RateLimit-* headers on 429 responses).
type stub struct {
	status  int
	body    string
	headers map[string]string
}

// newMux is a multi-route fake Braze server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 so a misrouted call fails loudly in tests.
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
			for k, v := range s.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
}

// run executes one braze subcommand against the fake server with the standard
// US DSN credential in env, returning the exit code and captured streams.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

// runResult is run returning the full Result (for CredentialRejected assertions).
func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	return runResultEnv(t, srv, map[string]string{EnvCredentials: testDSN}, args...)
}

// runResultEnv is runResult with a caller-chosen credential env (used for the
// malformed/missing-credential cases).
func runResultEnv(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.BaseURL = srv.URL
		svc.HC = srv.Client()
	}
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

// decodeBody unmarshals a captured request body into a generic map.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, raw)
	}
	return m
}

// errorEnvelope decodes a --json error line into its {"error":{...}} payload.
func errorEnvelope(t *testing.T, stderr string) map[string]any {
	t.Helper()
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	return env.Error
}
