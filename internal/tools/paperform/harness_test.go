package paperform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one request the fake Paperform server received.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Accept string
	Query  url.Values
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status  int
	body    string
	headers map[string]string
}

// newMux is a multi-route fake Paperform server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 so callers can assert on missing routes.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"),
			Query:  r.URL.Query(),
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
		_, _ = w.Write([]byte(`{"status":"error","message":"not found"}`))
	}))
}

// run invokes the service against a fake server and returns stdout, stderr, and
// the execution result.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf writeBuffer
	svc := &Service{
		// Mirror production where the base carries the /v1 version segment, so
		// paths in the service stay version-relative and requests land on
		// /v1/<resource>.
		BaseURL: srv.URL + "/v1",
		HC:      srv.Client(),
		Out:     &out,
		Err:     &errBuf,
	}
	env := map[string]string{EnvAPIKey: "test-key"}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return out.String(), errBuf.String(), res
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

// writeBuffer is a minimal io.Writer backed by a byte slice (avoids importing
// bytes into every test).
type writeBuffer struct{ b []byte }

func (w *writeBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func (w *writeBuffer) String() string { return string(w.b) }
