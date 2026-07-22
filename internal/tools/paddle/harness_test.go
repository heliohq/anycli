package paddle

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

// capturedRequest records one request the fake Paddle server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Version     string
	Accept      string
	ContentType string
	Query       map[string][]string
	Body        []byte
}

type stub struct {
	status  int
	body    string
	headers map[string]string
}

// newMux is a fake Paddle server answering each request from routes keyed by
// "METHOD /path" and recording every request. An unmatched route returns 404
// with a Paddle-shaped error body.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Version:     r.Header.Get("Paddle-Version"),
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
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"entity_not_found","detail":"not found"},"meta":{"request_id":"req_404"}}`))
	}))
}

// runPaddle executes one paddle invocation against srv, returning stdout,
// stderr and the execution result.
func runPaddle(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	svc.SetBaseURL(srv.URL)
	if env == nil {
		env = map[string]string{EnvToken: "pdl_live_apikey_test"}
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a non-nil error (should be nil for tool failures): %v", err)
	}
	return out.String(), errBuf.String(), res
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("output is not a JSON object: %v (%q)", err, s)
	}
	return m
}
