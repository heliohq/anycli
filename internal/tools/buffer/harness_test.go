package buffer

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

// capturedRequest records what the fake Buffer GraphQL server saw. Buffer is a
// single POST endpoint, so tests assert on the decoded body (query + variables)
// rather than on a path.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Body        []byte
}

// gqlBody is the decoded GraphQL request body.
type gqlBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// newServer returns a single-route GraphQL fake that records the request and
// replies with the given status/response for every POST.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// decodeReqBody unmarshals a captured GraphQL request body.
func decodeReqBody(t *testing.T, raw []byte) gqlBody {
	t.Helper()
	var b gqlBody
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, raw)
	}
	return b
}

// decodeOut unmarshals stdout JSON into a generic map.
func decodeOut(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, out)
	}
	return m
}
