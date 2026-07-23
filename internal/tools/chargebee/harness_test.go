package chargebee

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	testAPIKey = "test_key_live_abc123"
	testSite   = "acme-test"
)

// capturedRequest records what the fake Chargebee server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Query       string
	Accept      string
	Auth        string
	ContentType string
	Body        []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Accept:      r.Header.Get("Accept"),
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return server
}

// run executes one chargebee invocation against srv with the test credentials
// and returns the exit code plus captured stdout/stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf strings.Builder
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{
		EnvAPIKey: testAPIKey,
		EnvSite:   testSite,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// decodeBasicUser returns the username decoded from an "Authorization: Basic …"
// header — Chargebee sends the API key as the username with an empty password.
func decodeBasicUser(t *testing.T, header string) (user, pass string) {
	t.Helper()
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		t.Fatalf("Authorization header %q is not Basic", header)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		t.Fatalf("decode basic auth: %v", err)
	}
	user, pass, _ = strings.Cut(string(raw), ":")
	return user, pass
}
