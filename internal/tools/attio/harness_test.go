package attio

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records one request the fake Attio server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Accept      string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route (path only, no query).
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Attio server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 with a not_found error envelope.
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
		_, _ = w.Write([]byte(`{"status_code":404,"type":"invalid_request_error","code":"not_found","message":"not found"}`))
	}))
}

// runService executes one attio invocation against srv, capturing stdout/stderr
// and the exit result. token defaults to a test bearer.
func runService(t *testing.T, srv *httptest.Server, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := s.Execute(context.Background(), args, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned a transport-level error: %v", err)
	}
	return out.String(), errBuf.String(), res.ExitCode
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

// bodyMap decodes a request body into a generic JSON object.
func bodyMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, b)
	}
	return m
}

// dataMap decodes a request body and returns its "data" object.
func dataMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	m := bodyMap(t, b)
	d, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("body has no data object: %s", b)
	}
	return d
}

// okData is a minimal success envelope for a route stub.
func okData(inner string) stub {
	return stub{status: http.StatusOK, body: `{"data":` + inner + `}`}
}
