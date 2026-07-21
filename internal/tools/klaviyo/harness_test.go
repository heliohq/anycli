package klaviyo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records what the fake Klaviyo server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Query       url.Values
	Auth        string
	Revision    string
	Accept      string
	ContentType string
	Body        []byte
}

// newServer is a single-route fake that records the request and replies with a
// fixed status/body for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = capture(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func capture(r *http.Request) capturedRequest {
	body, _ := io.ReadAll(r.Body)
	return capturedRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.Query(),
		Auth:        r.Header.Get("Authorization"),
		Revision:    r.Header.Get("revision"),
		Accept:      r.Header.Get("Accept"),
		ContentType: r.Header.Get("Content-Type"),
		Body:        body,
	}
}

// run executes one command against srv with the default bearer token and
// returns exit code, stdout, and stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	return runWithToken(t, srv, "tok-123", args...)
}

// runWithToken executes one command with a caller-chosen credential (used to
// exercise the pk_ private-key auth scheme).
func runWithToken(t *testing.T, srv *httptest.Server, token string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: token})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

// bodyMap decodes a request body into a generic JSON object.
func bodyMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, b)
	}
	return m
}
