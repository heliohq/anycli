package tiktok

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Auth        string
	ContentType string
	rangeHeader string
	Body        []byte
}

func captureRequest(t *testing.T, r *http.Request) capturedRequest {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return capturedRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		RawQuery:    r.URL.RawQuery,
		Auth:        r.Header.Get("Authorization"),
		ContentType: r.Header.Get("Content-Type"),
		rangeHeader: r.Header.Get("Content-Range"),
		Body:        body,
	}
}

// serverURL reconstructs the base URL of the httptest server from an inbound
// request, so a handler can hand back an upload_url that points at itself.
func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func run(t *testing.T, server *httptest.Server, env map[string]string, args ...string) (int, string, string) {
	result, stdout, stderr := runResult(t, server, env, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, server *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	svc := &Service{
		APIBase: server.URL,
		HC:      server.Client(),
		Out:     &stdout,
		Err:     &stderr,
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, stdout.String(), stderr.String()
}

func fullEnv() map[string]string {
	return map[string]string{
		EnvAccessToken: "act.user-token",
		EnvOpenID:      "open-id-123",
	}
}

func jsonResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// okEnvelope wraps a data payload with the success error object TikTok returns.
func okEnvelope(data string) string {
	return `{"data":` + data + `,"error":{"code":"ok","message":"","log_id":"x"}}`
}
