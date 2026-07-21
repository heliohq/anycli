package pandadoc

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

// capturedRequest records what the fake PandaDoc server saw for one request.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status  int
	body    string
	rawBody []byte // when set, written verbatim (used for binary download bodies)
}

// newMux is a multi-route fake PandaDoc server: it answers each request from
// routes keyed by "METHOD /path" and appends every request to reqs. An
// unmatched route returns 404 with a PandaDoc-style error body.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		})
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			if len(s.rawBody) > 0 {
				w.Header().Set("Content-Type", "application/pdf")
				w.WriteHeader(s.status)
				_, _ = w.Write(s.rawBody)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"object_not_found","detail":"not found"}`))
	}))
}

// newServer answers every request with one status + response, recording the
// last request into got. Used by single-call tests.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes with a Bearer access token (the production credential path).
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	result, stdout, stderr := runEnv(t, srv, map[string]string{EnvAccessToken: "tok-abc"}, args...)
	return result.ExitCode, stdout, stderr
}

// runEnv executes with a caller-supplied credential env map.
func runEnv(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		// The real base carries the /public/v1 prefix; mirror it so recorded
		// request paths match the production path structure.
		svc.BaseURL = srv.URL + "/public/v1"
		svc.HC = srv.Client()
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// findReq returns the first recorded request matching method+path, or nil.
func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

// countReq counts recorded requests matching method+path.
func countReq(reqs []capturedRequest, method, path string) int {
	n := 0
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			n++
		}
	}
	return n
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
