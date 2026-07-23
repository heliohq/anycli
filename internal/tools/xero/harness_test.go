package xero

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

// capturedRequest records one request the fake Xero server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Tenant      string
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

// newMux is a multi-route fake Xero server keyed by "METHOD /path". It records
// every request into reqs. An unmatched route returns 404 so callers can assert
// on the not-found path.
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Tenant:      r.Header.Get("Xero-Tenant-Id"),
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
		_, _ = w.Write([]byte(`{"Type":"NotFound","Detail":"not found"}`))
	}))
}

// run executes one xero command against srv and returns stdout, stderr, and the
// result. token defaults to a non-empty test token unless overridden.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, execution.Result) {
	t.Helper()
	return runWithEnv(t, srv, map[string]string{"access_token": "tok-test"}, args...)
}

func runWithEnv(t *testing.T, srv *httptest.Server, cred map[string]string, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errb bytes.Buffer
	svc := &Service{Out: &out, Err: &errb}
	if srv != nil {
		svc.BaseURL = srv.URL
	}
	env := map[string]string{}
	if tok := cred["access_token"]; tok != "" {
		env[EnvToken] = tok
	}
	if ten := cred["tenant"]; ten != "" {
		env[EnvTenant] = ten
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return out.String(), errb.String(), res
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
