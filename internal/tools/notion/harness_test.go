package notion

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Notion server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. An
// unmatched route returns 404 object_not_found so the fetch/parent probes fall
// through the endpoint chain exactly as they do against the real API.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Auth:    r.Header.Get("Authorization"),
			Version: r.Header.Get("Notion-Version"),
			Query:   r.URL.Query(),
			Body:    body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"object":"error","code":"object_not_found","message":"not found"}`))
	}))
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

// countReq counts recorded requests matching method+path.
func countReq(reqs []capturedRequest, method, path string) int {
	n := 0
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			n++
		}
	}
	return n
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
