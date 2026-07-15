package figma

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
	Method        string
	Path          string
	RequestURI    string
	Query         string
	FigmaToken    string
	Authorization string
	ContentType   string
	Body          []byte
}

func newTestServer(t *testing.T, status int, response string, headers map[string]string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		*got = capturedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			RequestURI:    r.RequestURI,
			Query:         r.URL.RawQuery,
			FigmaToken:    r.Header.Get("X-Figma-Token"),
			Authorization: r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
			Body:          body,
		}
		for key, value := range headers {
			w.Header().Set(key, value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func runService(t *testing.T, server *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	result, stdout, stderr := runServiceResult(t, server, args...)
	return result.ExitCode, stdout, stderr
}

func runServiceResult(t *testing.T, server *httptest.Server, args ...string) (result execution.Result, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	service := &Service{BaseURL: server.URL, HC: server.Client(), Out: &out, Err: &errOut}
	result, err := service.Execute(context.Background(), args, map[string]string{EnvAccessToken: "figd_test_token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errOut.String()
}
