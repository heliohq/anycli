package posthog

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

const testToken = "phx_test_token"

// capturedRequest records what a fake PostHog server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Accept string
	Auth   string
	Body   []byte
}

// route is one canned reply on a recording server.
type route struct {
	status   int
	response string
}

// recordingServer routes by exact URL path, recording every hit into captured
// (keyed by path). Unmatched paths reply 404.
func recordingServer(t *testing.T, routes map[string]route, captured map[string]capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured[r.URL.Path] = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Accept: r.Header.Get("Accept"),
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		h, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"invalid_request","code":"not_found","detail":"Not found."}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(h.status)
		_, _ = w.Write([]byte(h.response))
	}))
}

// singleRouteServer replies with one status/body for every path and records the
// last request into got.
func singleRouteServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes args against a service whose BaseURL is srv (region probe
// disabled), with the standard test token in env.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	return runService(t, svc, map[string]string{EnvAccessToken: testToken}, args...)
}

// runService executes args against a caller-configured service and env.
func runService(t *testing.T, svc *Service, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc.Out = &out
	svc.Err = &errBuf
	if svc.HC == nil {
		svc.HC = http.DefaultClient
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, raw)
	}
	return m
}

func decodeErrorEnvelope(t *testing.T, stderr string) map[string]any {
	t.Helper()
	var envelope struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	return envelope.Error
}

// assertCredentialRejected runs args and asserts exit 1 with the credential
// marked rejected (the 401 → stale-credential feedback path).
func assertCredentialRejected(t *testing.T, svc *Service, env map[string]string, args ...string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc.Out = &out
	svc.Err = &errBuf
	if svc.HC == nil {
		svc.HC = http.DefaultClient
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection (stderr=%q)", result, errBuf.String())
	}
}
