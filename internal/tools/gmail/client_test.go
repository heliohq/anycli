package gmail

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

// seqStep is one scripted response for a scripted server.
type seqStep struct {
	status int
	body   string
}

// scripted serves canned responses in request order (the last step repeats)
// regardless of path; used for retry-path tests where the same route must
// answer differently per attempt.
type scripted struct {
	srv    *httptest.Server
	calls  int
	sleeps []time.Duration
}

func newScripted(t *testing.T, steps ...seqStep) *scripted {
	t.Helper()
	sc := &scripted{}
	sc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := sc.calls
		if i >= len(steps) {
			i = len(steps) - 1
		}
		sc.calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(steps[i].status)
		_, _ = w.Write([]byte(steps[i].body))
	}))
	t.Cleanup(sc.srv.Close)
	return sc
}

func (sc *scripted) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL: sc.srv.URL + "/gmail/v1",
		HC:      sc.srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep:   func(d time.Duration) { sc.sleeps = append(sc.sleeps, d) },
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func TestCallRetry_GETEmptyBodyThenSuccess(t *testing.T) {
	sc := newScripted(t,
		seqStep{http.StatusOK, ""},
		seqStep{http.StatusOK, `{"labels":[{"id":"INBOX","name":"INBOX","type":"system"}]}`},
	)
	result, stdout, stderr := sc.run(t, "labels", "list")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	if sc.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry after the empty 2xx body)", sc.calls)
	}
	if len(sc.sleeps) != 1 || sc.sleeps[0] != 200*time.Millisecond {
		t.Errorf("sleeps = %v, want [200ms]", sc.sleeps)
	}
	if !strings.Contains(stdout, "INBOX") {
		t.Errorf("stdout = %q, want the label list from the retried response", stdout)
	}
}

func TestCallRetry_GET5xxAnd429ThenSuccess(t *testing.T) {
	sc := newScripted(t,
		seqStep{http.StatusInternalServerError, `{"error":{"status":"INTERNAL","message":"boom"}}`},
		seqStep{http.StatusTooManyRequests, `{"error":{"status":"RESOURCE_EXHAUSTED","message":"slow down"}}`},
		seqStep{http.StatusOK, `{"emailAddress":"me@example.com","messagesTotal":1,"threadsTotal":1,"historyId":"1"}`},
	)
	result, stdout, stderr := sc.run(t, "profile")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	if sc.calls != 3 {
		t.Errorf("calls = %d, want 3", sc.calls)
	}
	want := []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}
	if len(sc.sleeps) != len(want) || sc.sleeps[0] != want[0] || sc.sleeps[1] != want[1] {
		t.Errorf("sleeps = %v, want %v", sc.sleeps, want)
	}
	if !strings.Contains(stdout, "me@example.com") {
		t.Errorf("stdout = %q, want the profile from the final response", stdout)
	}
}

func TestCallRetry_GETEmptyBodyExhaustedFails(t *testing.T) {
	sc := newScripted(t, seqStep{http.StatusOK, ""})
	result, _, stderr := sc.run(t, "labels", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if sc.calls != 3 {
		t.Errorf("calls = %d, want 3 (initial + 2 retries)", sc.calls)
	}
	if !strings.Contains(stderr, "empty response") {
		t.Errorf("stderr = %q, want an explicit empty-response error", stderr)
	}
}

func TestCallRetry_POSTNotRetried(t *testing.T) {
	sc := newScripted(t, seqStep{http.StatusInternalServerError, `{"error":{"status":"INTERNAL","message":"boom"}}`})
	result, _, stderr := sc.run(t, "messages", "modify", "m1", "--add-label", "STARRED")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if sc.calls != 1 {
		t.Errorf("calls = %d, want exactly 1 (POST must never be auto-retried)", sc.calls)
	}
	if len(sc.sleeps) != 0 {
		t.Errorf("sleeps = %v, want none", sc.sleeps)
	}
	if !strings.Contains(stderr, "HTTP 500") {
		t.Errorf("stderr = %q, want the 5xx error surfaced", stderr)
	}
}

func TestSanitizeJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean body untouched", `{"a":"b"}`, `{"a":"b"}`},
		{"control chars in string escaped", "{\"s\":\"a\r\n\x0b\x1fb\"}", `{"s":"a\u000d\u000a\u000b\u001fb"}`},
		{"whitespace outside strings kept", "{\n  \"a\": \"b\"\n}", "{\n  \"a\": \"b\"\n}"},
		{"escaped quote does not end string", "{\"s\":\"a\\\"\x1fb\"}", `{"s":"a\"\u001fb"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(sanitizeJSON([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("sanitizeJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("sanitizeJSON(%q) = %q, still not valid JSON", tc.in, got)
			}
		})
	}
}

func TestEmit_RefusesInvalidJSON(t *testing.T) {
	// A truncated 200 body that sanitization cannot repair must fail fast
	// instead of emitting garbage on --json.
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/profile": {http.StatusOK, `{"emailAddress":`},
	})
	result, stdout, stderr := f.run(t, "profile", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want no output when the provider body is unparseable", stdout)
	}
	if !strings.Contains(stderr, "invalid JSON") {
		t.Errorf("stderr = %q, want the invalid-JSON error", stderr)
	}
}
