package paypal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records one request the fake PayPal server received.
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

// newMux is a multi-route fake PayPal server: it answers each request from
// routes keyed by "METHOD /path" and records every request into reqs. The
// token endpoint (POST /v1/oauth2/token) is always answered with a canned
// bearer unless the caller overrides it, so data-call tests need not restate
// it. An unmatched route returns 404.
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
		if key == "POST /v1/oauth2/token" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"A-TEST-BEARER","token_type":"Bearer","expires_in":32400,"app_id":"APP-TEST"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"name":"RESOURCE_NOT_FOUND","message":"not found"}`))
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
