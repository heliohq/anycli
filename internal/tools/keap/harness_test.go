package keap

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

// capturedRequest records one request the fake Keap server received.
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

// newMux is a multi-route fake Keap server. It answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 so a mis-shaped request surfaces as a failure.
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
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
}

// runResult is the outcome of one in-test keap invocation.
type runResult struct {
	exitCode int
	rejected bool
	stdout   string
	stderr   string
}

// run executes one keap command against srv with the given token env, capturing
// stdout, stderr, and the execution result. A nil srv means no BaseURL override.
func run(t *testing.T, srv *httptest.Server, token string, args ...string) runResult {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.BaseURL = srv.URL
	}
	env := map[string]string{}
	if token != "" {
		env[EnvAccessToken] = token
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return runResult{exitCode: res.ExitCode, rejected: res.CredentialRejected, stdout: out.String(), stderr: errBuf.String()}
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
