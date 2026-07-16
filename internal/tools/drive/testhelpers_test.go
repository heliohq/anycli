package drive

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// newFixtureFunc is a fixture backed by an arbitrary handler: the handler
// returns true once it has written the response, false to fall through to a
// recorded 404. Every request is recorded before the handler runs. Used for
// flows that route on query params or need response headers (download's
// alt=media, resumable's Location).
func newFixtureFunc(t *testing.T, handler func(f *fixture, w http.ResponseWriter, r *http.Request) bool) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body.Bytes(),
		})
		if handler(f, w, r) {
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// decodeQ extracts the decoded `q` parameter from a recorded raw query string.
func decodeQ(t *testing.T, rawQuery string) string {
	t.Helper()
	v, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", rawQuery, err)
	}
	return v.Get("q")
}

// writeTemp writes content to dir/name and returns the full path.
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
