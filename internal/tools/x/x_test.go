package x

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Auth        string
	ContentType string
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
		Body:        body,
	}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func run(t *testing.T, server *httptest.Server, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	svc := &Service{
		APIBase: server.URL,
		HC:      server.Client(),
		Out:     &stdout,
		Err:     &stderr,
	}
	code, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return code, stdout.String(), stderr.String()
}

func fullEnv() map[string]string {
	return map[string]string{
		EnvAccessToken: "x-user-token",
		EnvUserID:      "2244994945",
	}
}

func jsonResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
