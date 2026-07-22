package later

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const testCredentials = "cid-123:secret-xyz"

// capturedRequest records what the fake reporting server saw on one route.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// fakeServer is a multi-route Later Influence reporting server. Each route
// returns its queued (status, body); /oauth/token issues a JWT so data routes
// can assert the Bearer header. Requests are recorded per path (last write
// wins) and counted so tests can assert the re-mint behavior.
type fakeServer struct {
	mu       sync.Mutex
	routes   map[string]routeReply
	captured map[string]capturedRequest
	hits     map[string]int
}

type routeReply struct {
	status int
	body   string
}

func newFakeServer(t *testing.T, routes map[string]routeReply) (*httptest.Server, *fakeServer) {
	t.Helper()
	f := &fakeServer{
		routes:   routes,
		captured: map[string]capturedRequest{},
		hits:     map[string]int{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.captured[r.URL.Path] = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		f.hits[r.URL.Path]++
		reply, ok := f.routes[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func (f *fakeServer) get(path string) (capturedRequest, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.captured[path], f.hits[path]
}

// countingServer models an expired-token session: the data route 401s on its
// first hit and succeeds on the second, so the service must re-mint exactly
// once. Token and data hits are counted for assertions.
type countingServer struct {
	mu        sync.Mutex
	tokenHits int
	dataHits  int
}

func newCountingServer(t *testing.T, dataBody string) (*httptest.Server, *countingServer) {
	t.Helper()
	c := &countingServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			c.mu.Lock()
			c.tokenHits++
			c.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jwt":"jwt-abc"}`))
		case "/v2/instances":
			c.mu.Lock()
			c.dataHits++
			first := c.dataHits == 1
			c.mu.Unlock()
			if first {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"token expired"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(dataBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func runResult(t *testing.T, srv *httptest.Server, creds string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvCredentials: creds})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	result, stdout, stderr := runResult(t, srv, testCredentials, args...)
	return result.ExitCode, stdout, stderr
}
