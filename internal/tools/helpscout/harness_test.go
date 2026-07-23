package helpscout

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Help Scout server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Accept string
	Auth   string
	Body   []byte
}

// routeHandler describes one route's canned reply on a multi-route server.
type routeHandler struct {
	status   int
	response string
	header   map[string]string
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
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

// newHeaderServer is newServer plus response headers (e.g. Resource-Id).
func newHeaderServer(t *testing.T, status int, response string, headers map[string]string, got *capturedRequest) *httptest.Server {
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
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// newMultiServer routes by exact URL path; each hit records into captured[path].
func newMultiServer(t *testing.T, routes map[string]routeHandler, captured map[string]capturedRequest) *httptest.Server {
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
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		for k, v := range h.header {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(h.status)
		_, _ = w.Write([]byte(h.response))
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

// decodeBody unmarshals a captured request body into a generic map.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, raw)
	}
	return m
}

// decodeArray unmarshals a captured request body into a generic slice.
func decodeArray(t *testing.T, raw []byte) []any {
	t.Helper()
	var a []any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("request body not a JSON array: %v (%s)", err, raw)
	}
	return a
}

// parseQuery parses a raw query string into url.Values.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}
