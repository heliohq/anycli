package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	testAccessJwt = "access-jwt-1"
	testDID       = "did:plc:alice"
	testHandle    = "alice.bsky.social"
)

// defaultSessionBody is the canned createSession response used unless a test
// overrides it.
const defaultSessionBody = `{"accessJwt":"access-jwt-1","refreshJwt":"refresh-jwt-1","did":"did:plc:alice","handle":"alice.bsky.social"}`

type recordedRequest struct {
	NSID        string
	Method      string
	Auth        string
	ContentType string
	Query       string
	Body        []byte
}

type stubResponse struct {
	status int
	body   string
}

// stub is a fake XRPC server. It records every request and serves per-NSID
// response queues; createSession is served with a canned session unless the
// test configures it.
type stub struct {
	t        *testing.T
	mu       sync.Mutex
	requests []recordedRequest
	seq      map[string][]stubResponse
}

func newStub(t *testing.T) *stub {
	return &stub{t: t, seq: map[string][]stubResponse{}}
}

// on queues one or more responses for an NSID, served in order (the last one
// repeats once exhausted).
func (s *stub) on(nsid string, responses ...stubResponse) {
	s.seq[nsid] = append(s.seq[nsid], responses...)
}

func ok(body string) stubResponse { return stubResponse{status: http.StatusOK, body: body} }
func fail(status int, body string) stubResponse {
	return stubResponse{status: status, body: body}
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	nsid := strings.TrimPrefix(r.URL.Path, "/xrpc/")

	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{
		NSID:        nsid,
		Method:      r.Method,
		Auth:        r.Header.Get("Authorization"),
		ContentType: r.Header.Get("Content-Type"),
		Query:       r.URL.RawQuery,
		Body:        body,
	})
	resp := s.next(nsid)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	_, _ = w.Write([]byte(resp.body))
}

func (s *stub) next(nsid string) stubResponse {
	queue := s.seq[nsid]
	switch {
	case len(queue) == 0:
		if nsid == "com.atproto.server.createSession" {
			return ok(defaultSessionBody)
		}
		return ok("{}")
	case len(queue) == 1:
		return queue[0]
	default:
		s.seq[nsid] = queue[1:]
		return queue[0]
	}
}

func (s *stub) count(nsid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, req := range s.requests {
		if req.NSID == nsid {
			n++
		}
	}
	return n
}

// last returns the most recent recorded request for an NSID.
func (s *stub) last(nsid string) recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.requests) - 1; i >= 0; i-- {
		if s.requests[i].NSID == nsid {
			return s.requests[i]
		}
	}
	s.t.Fatalf("no request recorded for %q", nsid)
	return recordedRequest{}
}

func fullEnv() map[string]string {
	return map[string]string{
		EnvCredentials: testHandle + ":app-pass-1234",
	}
}

func runStub(t *testing.T, s *stub, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	svc := &Service{APIBase: server.URL, HC: server.Client(), Out: &stdout, Err: &stderr}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, stdout.String(), stderr.String()
}

func writeFile(t *testing.T, path string, data []byte) error {
	t.Helper()
	return os.WriteFile(path, data, 0o600)
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		t.Fatalf("decode output %q: %v", raw, err)
	}
	return out
}
