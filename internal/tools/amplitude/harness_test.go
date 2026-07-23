package amplitude

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// testCreds is the default injected credential (apiKey:secretKey pre-image).
const (
	testAPIKey    = "apikey123"
	testSecretKey = "s3cr3t-value"
	testCreds     = testAPIKey + ":" + testSecretKey
)

// wantAuthHeader is the expected Authorization header for testCreds.
func wantAuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(testCreds))
}

// capturedRequest records what a fake Amplitude server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// runWith executes the amplitude service against srv with the given credentials.
func runWith(t *testing.T, srv *httptest.Server, creds string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvCredentials: creds})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// run is runWith with the default good credentials.
func run(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	return runWith(t, srv, testCreds, args...)
}

// capturingRT records the outbound request and returns a canned reply without
// touching the network — used to assert region host selection when no BaseURL
// override is set.
type capturingRT struct {
	got    *http.Request
	status int
	body   string
}

func (rt *capturingRT) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.got = r
	return &http.Response{
		StatusCode: rt.status,
		Body:       io.NopCloser(bytes.NewReader([]byte(rt.body))),
		Header:     make(http.Header),
	}, nil
}

// parseQuery parses a raw query string into url.Values.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}
