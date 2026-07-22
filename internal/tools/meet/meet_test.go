package meet

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// decodeQuery parses a recorded raw query string into a flat map (last value
// wins), decoding percent-escapes so tests can assert on the raw filter text.
func decodeQuery(raw string) (map[string]string, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			out[k] = v[len(v)-1]
		}
	}
	return out, nil
}

// recordedRequest is one request the fake Meet server saw.
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

// fixture is a fake Meet API server: routes keyed by "METHOD /v2/...", every
// request recorded in order. Retry backoff sleeps are recorded instead of
// slept so tests stay fast and deterministic.
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
		BaseURL: f.srv.URL + "/v2",
		HC:      f.srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep:   func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
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
	result, err := svc.Execute(context.Background(), []string{"records", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MEET_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"records", "explode"}, "explode"},
		{"records get without id", []string{"records", "get"}, "accepts 1 arg"},
		{"spaces update without flags", []string{"spaces", "update", "spaces/x"}, "nothing to update"},
		{"bad access type", []string{"spaces", "create", "--access-type", "public"}, "must be open, trusted, or restricted"},
		{"bad on/off", []string{"spaces", "create", "--auto-recording", "yes"}, "must be on or off"},
		{"text on non-transcript", []string{"transcripts", "text", "conferenceRecords/r1"}, "is not a transcript resource name"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestScopeHintOn403(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "records", "list")
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
		{"server failure", http.StatusInternalServerError, "INTERNAL", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v2/spaces/x": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "spaces", "get", "spaces/x")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestScopeHintAbsentOnPlainError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1": {http.StatusNotFound, `{"error":{"status":"NOT_FOUND","message":"expired"}}`},
	})
	_, _, stderr := f.run(t, "records", "get", "r1")
	if strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, scope hint must only appear on 401/403", stderr)
	}
}

func TestRetryOn503ForGET(t *testing.T) {
	// The fixture cannot vary a response per attempt, so drive the retry path
	// via a persistent 503: two backoffs recorded, then the final error.
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords": {http.StatusServiceUnavailable, `{"error":{"status":"UNAVAILABLE","message":"try later"}}`},
	})
	result, _, _ := f.run(t, "records", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if len(f.sleeps) != len(retryBackoffs) {
		t.Errorf("recorded %d backoff sleeps, want %d", len(f.sleeps), len(retryBackoffs))
	}
}

// TestSideEffectAnnotations asserts every runnable leaf command of the tree
// carries an explicit anycli.side_effect annotation with the reviewed value
// (design 318 may-mutate criterion), and that group commands carry none.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"meet records list":          "false", // GET /conferenceRecords
		"meet records get":           "false", // GET /conferenceRecords/{r}
		"meet participants list":     "false", // GET /conferenceRecords/{r}/participants
		"meet participants sessions": "false", // GET .../participantSessions
		"meet recordings list":       "false", // GET /conferenceRecords/{r}/recordings
		"meet transcripts list":      "false", // GET /conferenceRecords/{r}/transcripts
		"meet transcripts entries":   "false", // GET .../transcripts/{t}/entries
		"meet transcripts text":      "false", // synthetic read: GET participants + entries
		"meet smart-notes list":      "false", // GET v2beta .../smartNotes
		"meet spaces get":            "false", // GET /spaces/{s}
		"meet spaces create":         "true",  // POST /spaces
		"meet spaces update":         "true",  // PATCH /spaces/{s}
		"meet spaces end-conference": "true",  // POST /spaces/{s}:endActiveConference
	}

	root := (&Service{}).NewCommandTree()
	got := map[string]string{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		val, ok := cmd.Annotations["anycli.side_effect"]
		if cmd.HasSubCommands() {
			if ok {
				t.Errorf("%s: group command must not carry anycli.side_effect, got %q", cmd.CommandPath(), val)
			}
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		if !ok {
			t.Errorf("%s: runnable leaf missing explicit anycli.side_effect annotation", cmd.CommandPath())
			return
		}
		got[cmd.CommandPath()] = val
	}
	walk(root)

	for path, wantVal := range want {
		if gotVal, ok := got[path]; !ok {
			t.Errorf("%s: leaf command not found in tree", path)
		} else if gotVal != wantVal {
			t.Errorf("%s: anycli.side_effect = %q, want %q", path, gotVal, wantVal)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s: new runnable leaf not covered by this table — classify it per the design 318 may-mutate criterion", path)
		}
	}
}
