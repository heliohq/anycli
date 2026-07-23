package googleads

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

// capturedRequest records what the fake Google Ads server saw.
type capturedRequest struct {
	Method          string
	Path            string
	Query           string
	Accept          string
	ContentType     string
	Auth            string
	DeveloperToken  string
	LoginCustomerID string
	Body            []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:          r.Method,
			Path:            r.URL.Path,
			Query:           r.URL.RawQuery,
			Accept:          r.Header.Get("Accept"),
			ContentType:     r.Header.Get("Content-Type"),
			Auth:            r.Header.Get("Authorization"),
			DeveloperToken:  r.Header.Get("developer-token"),
			LoginCustomerID: r.Header.Get("login-customer-id"),
			Body:            body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes a google-ads subcommand against srv with both credentials seeded
// and returns the exit code plus captured streams.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, nil, args...)
	return result.ExitCode, stdout, stderr
}

// runResult executes with an optional extra env overlay (e.g. the login
// customer id) and returns the full Result for classification assertions.
func runResult(t *testing.T, srv *httptest.Server, extraEnv map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{
		EnvAccessToken:    "user-tok",
		EnvDeveloperToken: "dev-tok",
	}
	for k, v := range extraEnv {
		env[k] = v
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
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
