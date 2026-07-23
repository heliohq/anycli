package signnow

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records one request the fake SignNow server received.
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

// newMux is a multi-route fake SignNow server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 with a current-dialect error body.
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
		_, _ = w.Write([]byte(`{"errors":[{"code":65558,"message":"Unable to find a route to match the URI"}]}`))
	}))
}

// runSN executes one signnow invocation against srv with the fixed test token,
// returning the result plus captured stdout/stderr.
func runSN(t *testing.T, srv *httptest.Server, args ...string) (result execResult, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	r, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "secret"})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return execResult{exitCode: r.ExitCode, credentialRejected: r.CredentialRejected}, out.String(), errBuf.String()
}

// execResult is a test-local view of execution.Result (that package is
// internal to the parent tree; the fields we assert on are copied here).
type execResult struct {
	exitCode           int
	credentialRejected bool
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

// decodeStdout decodes the emitted stdout JSON into a generic object.
func decodeStdout(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v (%q)", err, s)
	}
	return m
}
