package freshservice

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

// testAPIKey is the API key carried in the test credential blob; asserted in
// the Basic Authorization header.
const testAPIKey = "key-abc123"

// testBlob is a well-formed FRESHSERVICE_URL for the fake account.
const testBlob = "https://" + testAPIKey + "@acme.freshservice.com"

// capturedRequest records what the fake Freshservice server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Accept string
	Auth   string
	Body   []byte
}

// fakeServer routes by exact path; each hit records into captured[path] and
// replies from routes[path]. Unrouted paths 404 with a Freshservice-shaped body.
type routeReply struct {
	status  int
	body    string
	headers map[string]string
}

func newFakeServer(t *testing.T, routes map[string]routeReply, captured map[string]capturedRequest) *httptest.Server {
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
		reply, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"description":"NOT_FOUND"}`))
			return
		}
		for k, v := range reply.headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		status := reply.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply.body))
	}))
}

// runBlob executes the service with an explicit credential blob and BaseURL
// override. Returns the exit code, stdout, stderr, and the credential-rejected
// flag.
func runBlob(t *testing.T, srv *httptest.Server, blob string, args ...string) (int, string, string, bool) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.BaseURL = srv.URL
		svc.HC = srv.Client()
	}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvURL: blob})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res.ExitCode, out.String(), errBuf.String(), res.CredentialRejected
}

// run executes with the standard test blob.
func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	t.Helper()
	code, out, errStr, _ := runBlob(t, srv, testBlob, args...)
	return code, out, errStr
}

// runResult exposes the full Result (for credential-rejection assertions).
func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf, BaseURL: srv.URL, HC: srv.Client()}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvURL: testBlob})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res, out.String(), errBuf.String()
}

// decodeJSON unmarshals stdout into a generic map.
func decodeJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, raw)
	}
	return m
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
