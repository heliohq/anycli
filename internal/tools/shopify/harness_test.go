package shopify

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

// capturedRequest records one request the fake Shopify GraphQL server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Accept      string
	Body        []byte
}

// newServer returns a fake GraphQL endpoint that records every request and
// answers with a single canned status + body.
func newServer(t *testing.T, reqs *[]capturedRequest, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get(accessTokenHeader),
			ContentType: r.Header.Get("Content-Type"),
			Accept:      r.Header.Get("Accept"),
			Body:        b,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// runResult bundles one invocation's observable outcome.
type runResult struct {
	stdout string
	stderr string
	result execution.Result
}

// runAgainst executes the shopify service against a fake server URL with a
// default seeded credential env.
func runAgainst(t *testing.T, srv *httptest.Server, args ...string) runResult {
	t.Helper()
	var out, errb bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errb}
	res, err := svc.Execute(context.Background(), args, map[string]string{
		EnvAccessToken: "shpat_test",
		EnvStore:       "myshop.myshopify.com",
	})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return runResult{stdout: out.String(), stderr: errb.String(), result: res}
}

// bodyOf decodes a captured GraphQL request body into query + variables.
func bodyOf(t *testing.T, b []byte) (string, map[string]any) {
	t.Helper()
	var env struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("request body is not a JSON object: %v (%s)", err, b)
	}
	return env.Query, env.Variables
}

// decodeJSON parses stdout JSON into a generic map.
func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v (%s)", err, s)
	}
	return m
}
