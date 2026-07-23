package googleanalytics

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

// recordedRequest is one request the fake GA server saw.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake server standing in for BOTH GA hosts (Data API and Admin
// API): routes keyed by "METHOD /path", every request recorded in order.
// Retry backoff sleeps are recorded instead of slept so tests stay fast and
// deterministic. The two base URLs point at distinct path prefixes on the
// same httptest server so cross-host routing mistakes surface as test
// failures, not silent passes.
type fixture struct {
	srv      *httptest.Server
	requests []recordedRequest
	sleeps   []time.Duration
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body.Bytes(),
		})
		rt, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// last returns the most recent request matching method+path.
func (f *fixture) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	t.Fatalf("no recorded request %s %s", method, path)
	return recordedRequest{}
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		DataBaseURL:  f.srv.URL + "/data/v1beta",
		AdminBaseURL: f.srv.URL + "/admin/v1beta",
		HC:           f.srv.Client(),
		Out:          &out,
		Err:          &errBuf,
		sleep:        func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
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
	result, err := svc.Execute(context.Background(), []string{"property", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "GOOGLE_ANALYTICS_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingTokenJSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"property", "list", "--json"}, map[string]string{})
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
	if uErr := json.Unmarshal(errBuf.Bytes(), &envelope); uErr != nil {
		t.Fatalf("stderr is not a JSON envelope: %q", errBuf.String())
	}
	if !strings.Contains(envelope.Error.Message, "GOOGLE_ANALYTICS_ACCESS_TOKEN") {
		t.Errorf("envelope message = %q, want the missing-token message", envelope.Error.Message)
	}
}

func TestExecute_UnknownSubcommandIsUsageError(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, _ := f.run(t, "nonsense")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", result.ExitCode)
	}
}

func TestExecute_BareGroupShowsHelpAndUnknownChildFails(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, _ := f.run(t, "report")
	if result.ExitCode != 0 {
		t.Errorf("bare group exit code = %d, want 0 (help)", result.ExitCode)
	}
	result, _, _ = f.run(t, "report", "nonsense")
	if result.ExitCode != 2 {
		t.Errorf("unknown group child exit code = %d, want 2", result.ExitCode)
	}
}

func TestUnauthorizedRejectsCredentialAndHintsScope(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {
			status: http.StatusUnauthorized,
			body:   `{"error":{"status":"UNAUTHENTICATED","message":"Request had invalid authentication credentials."}}`,
		},
	})
	result, _, stderr := f.run(t, "property", "list")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true on HTTP 401")
	}
	if !strings.Contains(stderr, "missing scope") {
		t.Errorf("stderr = %q, want the reconnect scope hint", stderr)
	}
}

func TestForbiddenHintsScopeButKeepsCredential(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {
			status: http.StatusForbidden,
			body:   `{"error":{"status":"PERMISSION_DENIED","message":"Google Analytics Admin API has not been used in project 123 before or it is disabled."}}`,
		},
	})
	result, _, stderr := f.run(t, "property", "list")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false on a plain 403")
	}
	if !strings.Contains(stderr, "missing scope") {
		t.Errorf("stderr = %q, want the reconnect scope hint", stderr)
	}
	if !strings.Contains(stderr, "PERMISSION_DENIED") {
		t.Errorf("stderr = %q, want the provider error surfaced", stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {
			status: http.StatusBadRequest,
			body:   `{"error":{"status":"INVALID_ARGUMENT","message":"bad page token"}}`,
		},
	})
	result, _, stderr := f.run(t, "property", "list", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %q", stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind api status 400", envelope.Error)
	}
	if !strings.Contains(envelope.Error.Message, "INVALID_ARGUMENT") {
		t.Errorf("envelope message = %q, want provider status surfaced", envelope.Error.Message)
	}
}

func TestGetRetriesOnTransientStatus(t *testing.T) {
	attempts := 0
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"accountSummaries":[]}`))
	}))
	t.Cleanup(f.srv.Close)
	stdout := f.runOK(t, "property", "list")
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (one retry)", attempts)
	}
	if len(f.sleeps) != 1 {
		t.Errorf("sleeps = %v, want exactly one backoff", f.sleeps)
	}
	if !strings.Contains(stdout, "no properties") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestPostIsNeverRetried(t *testing.T) {
	attempts := 0
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"status":"INTERNAL","message":"boom"}}`))
	}))
	t.Cleanup(f.srv.Close)
	result, _, _ := f.run(t, "report", "run", "--property", "123", "--metrics", "activeUsers")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no POST retry)", attempts)
	}
}

func TestJSONOutputRefusesInvalidProviderJSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK, body: `{not json`},
	})
	result, stdout, _ := f.run(t, "property", "list", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1 for invalid provider JSON", result.ExitCode)
	}
	if strings.Contains(stdout, "{not json") {
		t.Errorf("stdout = %q, must never carry invalid JSON", stdout)
	}
}

func TestSanitizeJSONEscapesControlCharsInsideStrings(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK,
			body: "{\"accountSummaries\":[{\"account\":\"accounts/1\",\"displayName\":\"bad\x01name\",\"propertySummaries\":[]}]}"},
	})
	stdout := f.runOK(t, "property", "list", "--json")
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout is not valid JSON: %q", stdout)
	}
	if !strings.Contains(stdout, "bad\\u0001name") {
		t.Errorf("stdout = %q, want the control char escaped in place", stdout)
	}
}
