package recurly

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one inbound request to the fake Recurly server.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Accept      string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status  int
	body    string
	headers map[string]string
}

// newMux is a multi-route fake Recurly server keyed by "METHOD /path". It
// records every request into reqs and returns a Recurly-shaped not_found error
// for unmatched routes.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			for k, v := range s.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"not_found","message":"not found"}}`))
	}))
}

// runService executes one recurly invocation against srv with the given key and
// region env, capturing stdout/stderr. It mirrors how the AnyCLI engine calls a
// built-in service.
func runService(t *testing.T, srv *httptest.Server, key, region string, args ...string) (stdout, stderr string, res execution.Result) {
	t.Helper()
	var out, errBuf writerBuf
	s := &Service{BaseURL: srv.URL, Out: &out, Err: &errBuf}
	env := map[string]string{}
	if key != "" {
		env[EnvKey] = key
	}
	if region != "" {
		env[EnvRegion] = region
	}
	res, err := s.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return out.String(), errBuf.String(), res
}

// basicAuth is the expected Authorization header for a private key (blank pass).
func basicAuth(key string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(key+":"))
}

// writerBuf is a tiny buffer implementing io.Writer + String (avoids importing
// bytes.Buffer at each call site while keeping the harness self-contained).
type writerBuf struct{ b []byte }

func (w *writerBuf) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *writerBuf) String() string              { return string(w.b) }

// findReq returns the first recorded request matching method+path, or nil.
func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}
