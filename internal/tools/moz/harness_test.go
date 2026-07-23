package moz

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

// capturedRequest records what the fake Moz JSON-RPC server saw.
type capturedRequest struct {
	Method      string // HTTP method
	Path        string
	Token       string // x-moz-token header
	ContentType string
	Body        []byte
}

// rpcBody is the decoded JSON-RPC envelope of a captured request body.
type rpcBody struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"` // JSON-RPC method name
	Params  struct {
		Data json.RawMessage `json:"data"`
	} `json:"params"`
}

// newServer returns a single-route JSON-RPC server that records the request and
// replies with the given HTTP status/body for every POST.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Token:       r.Header.Get("x-moz-token"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes one moz invocation against srv with a fixed token and returns
// the exit code and captured streams.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL:      srv.URL,
		HC:           srv.Client(),
		Out:          &out,
		Err:          &errBuf,
		newRequestID: func() string { return "test-request-id-000000000000000000" },
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIToken: "moz-tok-123"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// decodeRPC decodes a captured request body as a JSON-RPC envelope.
func decodeRPC(t *testing.T, raw []byte) rpcBody {
	t.Helper()
	var b rpcBody
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("request body is not a JSON-RPC envelope: %v (%s)", err, raw)
	}
	return b
}

// decodeData decodes a captured envelope's params.data into a generic map.
func decodeData(t *testing.T, b rpcBody) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b.Params.Data, &m); err != nil {
		t.Fatalf("params.data is not a JSON object: %v (%s)", err, b.Params.Data)
	}
	return m
}
