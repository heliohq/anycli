package searchconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// recordedRequest is one request the fake Search Console server saw. RawPath
// preserves the escaped form so tests can assert siteUrl/feedpath escaping.
type recordedRequest struct {
	Method  string
	RawPath string
	Auth    string
	Body    []byte
}

// route is a canned response for "METHOD /escaped-path".
type route struct {
	status int
	body   string
}

// fixture is a fake Search Console API server fronting BOTH bases: routes are
// keyed by "METHOD <escaped path>" — the webmasters v3 base mounts under
// /webmasters/v3 and the URL-inspection base under /v1, mirroring production.
type fixture struct {
	srv      *httptest.Server
	requests []recordedRequest
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method:  r.Method,
			RawPath: r.URL.EscapedPath(),
			Auth:    r.Header.Get("Authorization"),
			Body:    body.Bytes(),
		})
		rt, ok := routes[r.Method+" "+r.URL.EscapedPath()]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		if rt.body != "" {
			_, _ = w.Write([]byte(rt.body))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// last returns the most recent request matching method + escaped path.
func (f *fixture) last(t *testing.T, method, rawPath string) recordedRequest {
	t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].RawPath == rawPath {
			return f.requests[i]
		}
	}
	t.Fatalf("no recorded request %s %s", method, rawPath)
	return recordedRequest{}
}

// fixedNow is the deterministic clock injected into every test service:
// 2026-07-21 10:00 in America/Los_Angeles.
func fixedNow() time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 7, 21, 10, 0, 0, 0, loc)
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL:        f.srv.URL + "/webmasters/v3",
		InspectBaseURL: f.srv.URL + "/v1",
		HC:             f.srv.Client(),
		Out:            &out,
		Err:            &errBuf,
		now:            fixedNow,
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func (f *fixture) runOK(t *testing.T, args ...string) string {
	t.Helper()
	result, stdout, stderr := f.run(t, args...)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	return stdout
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"sites", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "SEARCH_CONSOLE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingTokenJSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"sites", "list", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(errBuf.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %q", errBuf.String())
	}
	if !strings.Contains(envelope.Error.Message, "SEARCH_CONSOLE_ACCESS_TOKEN") {
		t.Errorf("envelope message = %q, want the missing-token message", envelope.Error.Message)
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "sites", "explode")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 (stderr: %s)", result.ExitCode, stderr)
	}
	if !strings.Contains(stderr, "explode") {
		t.Errorf("stderr = %q, want the unknown subcommand named", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("usage failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestUsageErrors_Exit2(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"sites get without site", []string{"sites", "get"}, "--site"},
		{"sitemaps list without site", []string{"sitemaps", "list"}, "--site"},
		{"sitemaps get without sitemap", []string{"sitemaps", "get", "--site", "https://example.com/"}, "--sitemap"},
		{"sitemaps submit without sitemap", []string{"sitemaps", "submit", "--site", "https://example.com/"}, "--sitemap"},
		{"sitemaps delete without sitemap", []string{"sitemaps", "delete", "--site", "https://example.com/"}, "--sitemap"},
		{"query without site", []string{"query", "--start", "2026-06-01", "--end", "2026-06-30"}, "--site"},
		{"query without dates", []string{"query", "--site", "https://example.com/"}, "--start"},
		{"query start without end", []string{"query", "--site", "https://example.com/", "--start", "2026-06-01"}, "--end"},
		{"query days with start", []string{"query", "--site", "https://example.com/", "--days", "7", "--start", "2026-06-01"}, "mutually exclusive"},
		{"query bad filter", []string{"query", "--site", "https://example.com/", "--days", "7", "--filter", "query=pizza"}, "dimension:operator:expression"},
		{"inspect without url", []string{"inspect", "--site", "https://example.com/"}, "--url"},
		{"inspect without site", []string{"inspect", "--url", "https://example.com/page"}, "--site"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 2 {
				t.Fatalf("exit code = %d, want 2 (stderr: %s)", result.ExitCode, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("usage failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestAPIError_Exit1AndJSONEnvelope(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "sites", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}

	result, _, stderr = f.run(t, "sites", "list", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %q", stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != http.StatusForbidden {
		t.Errorf("envelope = %+v, want kind api status 403", envelope.Error)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "UNKNOWN", true},
		{"explicit unauthenticated status", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"rate limited", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /webmasters/v3/sites": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "sites", "list")
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestRateLimitSurfacedVerbatim(t *testing.T) {
	// Per-property inspection quota exhaustion must surface Google's 429
	// verbatim — no client-side throttling or retry masking it.
	f := newFixture(t, map[string]route{
		"POST /v1/urlInspection/index:inspect": {http.StatusTooManyRequests, `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Quota exceeded"}}`},
	})
	result, _, stderr := f.run(t, "inspect", "--site", "https://example.com/", "--url", "https://example.com/page")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "HTTP 429") || !strings.Contains(stderr, "Quota exceeded") {
		t.Errorf("stderr = %q, want the verbatim 429 quota error", stderr)
	}
	if len(f.requests) != 1 {
		t.Errorf("saw %d requests, want exactly 1 (no auto-retry)", len(f.requests))
	}
}
