package zuora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records one request the fake Zuora server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Accept      string
	ContentType string
	Query       url.Values
	Form        url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Zuora server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. The token
// endpoint (POST /oauth/token) is always answered with a canned bearer unless
// the caller overrides it, so data-call tests need not restate it. An unmatched
// route returns 404 with Zuora's lowercase resource-error envelope.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Form:        r.PostForm,
			Body:        body,
		})
		w.Header().Set("Content-Type", "application/json")
		key := r.Method + " " + r.URL.Path
		if s, ok := routes[key]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		if key == "POST /oauth/token" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"A-TEST-BEARER","token_type":"bearer","expires_in":3600,"scope":"read","jti":"jti-1"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"processId":"P1","reasons":[{"code":50000040,"message":"not found"}]}`))
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
