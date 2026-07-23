package iterable

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Iterable server saw.
type capturedRequest struct {
	Method  string
	Path    string
	Query   string
	Accept  string
	APIKey  string
	Auth    string
	Content string
	Body    []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.RawQuery,
			Accept:  r.Header.Get("Accept"),
			APIKey:  r.Header.Get("Api-Key"),
			Auth:    r.Header.Get("Authorization"),
			Content: r.Header.Get("Content-Type"),
			Body:    body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// runWithKey executes the service against srv with the given raw credential.
func runWithKey(t *testing.T, srv *httptest.Server, key string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: key})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res, out.String(), errBuf.String()
}

// runNoServer executes the service with no reachable server (credential-format
// paths that fail before any HTTP call).
func runNoServer(t *testing.T, key string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: "http://127.0.0.1:0", Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: key})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res, out.String(), errBuf.String()
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
