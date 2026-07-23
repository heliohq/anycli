package pennylane

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

// testToken is the canned Bearer access token every harness run injects.
const testToken = "pl-access-token"

// capturedRequest records one request the fake Pennylane server received.
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
	status int
	body   string
}

// newMux is a multi-route fake Pennylane server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 with a Pennylane-shaped error body.
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
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
}

// runResult is the outcome of one harness Execute call.
type runResult struct {
	res    execution.Result
	stdout string
	stderr string
	err    error
}

// run drives Service.Execute against srv with a real token in env and captured
// output streams. Pass token="" to exercise the missing-credential path.
func run(t *testing.T, srv *httptest.Server, token string, args ...string) runResult {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.BaseURL = srv.URL
		svc.HC = srv.Client()
	}
	env := map[string]string{}
	if token != "" {
		env[EnvToken] = token
	}
	res, err := svc.Execute(context.Background(), args, env)
	return runResult{res: res, stdout: out.String(), stderr: errBuf.String(), err: err}
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

// decodeErrorEnvelope parses the {"error":{…}} JSON envelope from stderr.
func decodeErrorEnvelope(t *testing.T, s string) map[string]any {
	t.Helper()
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, s)
	}
	return env.Error
}
