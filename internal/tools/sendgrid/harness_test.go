package sendgrid

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

// capturedRequest records what the fake SendGrid server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Accept string
	Auth   string
	Body   []byte
}

// serverResponse describes the canned reply the fake returns.
type serverResponse struct {
	status  int
	body    string
	headers map[string]string
}

// newServer returns a single-route server that records the request and replies
// with the given response for every path.
func newServer(t *testing.T, resp serverResponse, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Accept: r.Header.Get("Accept"),
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		for k, v := range resp.headers {
			w.Header().Set(k, v)
		}
		if resp.body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.status)
		if resp.body != "" {
			_, _ = w.Write([]byte(resp.body))
		}
	}))
}

// run executes one sendgrid command against the fake and returns exit code +
// captured streams, with the API key seeded in env.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: "SG.test-key"})
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
