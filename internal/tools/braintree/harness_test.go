package braintree

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one request the fake GraphQL server received.
type capturedRequest struct {
	Method      string
	Auth        string
	Version     string
	ContentType string
	Query       string
	Variables   map[string]any
}

// fakeServer is an httptest GraphQL fake: it records every request and answers
// from a body picked by the caller. status/body are the canned response.
type fakeServer struct {
	server *httptest.Server
	reqs   []capturedRequest
	status int
	body   string
}

func newFakeServer(t *testing.T, status int, body string) *fakeServer {
	t.Helper()
	fs := &fakeServer{status: status, body: body}
	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var env struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &env)
		fs.reqs = append(fs.reqs, capturedRequest{
			Method:      r.Method,
			Auth:        r.Header.Get("Authorization"),
			Version:     r.Header.Get("Braintree-Version"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       env.Query,
			Variables:   env.Variables,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fs.status)
		_, _ = w.Write([]byte(fs.body))
	}))
	t.Cleanup(fs.server.Close)
	return fs
}

// run executes one braintree invocation against the fake, returning stdout,
// stderr, and the result. The seeded credentials are the canonical test key
// pair; BaseURL points at the fake so the environment field is not consulted.
func (fs *fakeServer) run(t *testing.T, jsonMode bool, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: fs.server.URL, Out: &out, Err: &errBuf}
	full := args
	if jsonMode {
		full = append([]string{"--json"}, args...)
	}
	res, err := svc.Execute(context.Background(), full, testEnv())
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return out.String(), errBuf.String(), res
}

func testEnv() map[string]string {
	return map[string]string{
		EnvMerchantID:  "merch123",
		EnvPublicKey:   "pubkey123",
		EnvPrivateKey:  "privkey_SECRET",
		EnvEnvironment: "sandbox",
	}
}
