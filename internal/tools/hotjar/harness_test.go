package hotjar

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Hotjar server saw for one request.
type capturedRequest struct {
	Method      string
	Path        string
	Query       string
	Auth        string
	ContentType string
	Body        []byte
}

// cannedResponse is one queued reply for a path.
type cannedResponse struct {
	status   int
	response string
}

// fakeHotjar is a path-routed fake Hotjar API. Each path serves a queue of
// responses (popped per hit; the last reply repeats once the queue drains).
// Every request is recorded in order so tests can assert request shape.
type fakeHotjar struct {
	mu       sync.Mutex
	queues   map[string][]cannedResponse
	requests []capturedRequest
}

func newFake() *fakeHotjar {
	return &fakeHotjar{queues: map[string][]cannedResponse{}}
}

// on queues one or more responses for a path (served in order).
func (f *fakeHotjar) on(path string, responses ...cannedResponse) *fakeHotjar {
	f.queues[path] = append(f.queues[path], responses...)
	return f
}

func (f *fakeHotjar) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		})
		queue := f.queues[r.URL.Path]
		var reply cannedResponse
		switch {
		case len(queue) == 0:
			f.mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no canned response for ` + r.URL.Path + `"}`))
			return
		case len(queue) == 1:
			reply = queue[0] // last reply repeats
		default:
			reply = queue[0]
			f.queues[r.URL.Path] = queue[1:]
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.response))
	}))
}

// hits returns the recorded requests for a path in order.
func (f *fakeHotjar) hits(path string) []capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []capturedRequest
	for _, req := range f.requests {
		if req.Path == path {
			out = append(out, req)
		}
	}
	return out
}

// first returns the first recorded request for a path (fails if none).
func (f *fakeHotjar) first(t *testing.T, path string) capturedRequest {
	t.Helper()
	got := f.hits(path)
	if len(got) == 0 {
		t.Fatalf("no request recorded for %s", path)
	}
	return got[0]
}

// okToken is the standard successful client_credentials reply.
const okToken = `{"access_token":"tok-abc","token_type":"Bearer","expires_in":3600}`

// withToken queues a successful token exchange on the fake.
func (f *fakeHotjar) withToken() *fakeHotjar {
	return f.on(tokenPath, cannedResponse{http.StatusOK, okToken})
}

// runHotjar executes a hotjar command against the fake with both secrets set.
func runHotjar(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL: srv.URL,
		HC:      srv.Client(),
		Out:     &out,
		Err:     &errBuf,
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{
		EnvClientID:     "cid-1",
		EnvClientSecret: "csecret-1",
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func parseForm(t *testing.T, raw []byte) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatalf("bad form body %q: %v", raw, err)
	}
	return v
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}
