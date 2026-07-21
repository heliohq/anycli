package forms

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// recordedRequest is one request the fake API server saw.
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

// fixture is a fake Forms + Drive API server: routes keyed by "METHOD /path",
// every request recorded in order. Retry backoff sleeps are recorded instead of
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
		BaseURL:      f.srv.URL + "/v1",
		DriveBaseURL: f.srv.URL + "/drive/v3",
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
	result, err := svc.Execute(context.Background(), []string{"get", "f1"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "FORMS_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"explode"}, "explode"},
		{"unknown responses sub", []string{"responses", "explode"}, "explode"},
		{"create without title", []string{"create"}, "--title is required"},
		{"get without id", []string{"get"}, "accepts 1 arg"},
		{"batch-update no requests", []string{"batch-update", "f1"}, "provide --requests"},
		{"batch-update both sources", []string{"batch-update", "f1", "--requests", "[]", "--requests-file", "x"}, "none of the others can be"},
		{"responders add no target", []string{"responders", "add", "f1"}, "pass --anyone or --to"},
		{"responders add both targets", []string{"responders", "add", "f1", "--anyone", "--to", "a@b.c"}, "only one of --anyone or --to"},
		{"responder link rejected", []string{"get", "https://docs.google.com/forms/d/e/ABC123/viewform"}, "responder link"},
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
		"GET /v1/forms/f1": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "get", "f1")
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
		{"rate limited retried then fails", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1/forms/f1": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "get", "f1")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestGet_HumanAndJSON(t *testing.T) {
	body := `{"formId":"f1","info":{"title":"Survey","documentTitle":"Doc"},"responderUri":"https://forms.gle/x","linkedSheetId":"sheet9","publishSettings":{"publishState":{"isPublished":true,"isAcceptingResponses":true}},"items":[{"itemId":"i1","title":"Q1"}]}`
	f := newFixture(t, map[string]route{
		"GET /v1/forms/f1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "get", "f1")
	for _, want := range []string{"FormId:        f1", "Title:         Survey", "Published:     true", "Accepting:     true", "LinkedSheet:   sheet9", "Items:         1", "Q1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want it to contain %q", stdout, want)
		}
	}
	got := f.last(t, "GET", "/v1/forms/f1")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}

	stdout = f.runOK(t, "get", "f1", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestGet_AcceptsEditURL(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/forms/f1": {http.StatusOK, `{"formId":"f1","info":{"title":"T"}}`},
	})
	f.runOK(t, "get", "https://docs.google.com/forms/d/f1/edit")
	f.last(t, "GET", "/v1/forms/f1") // extracted the bare id from the edit URL
}

// TestSideEffectAnnotations asserts every runnable leaf command of the tree
// carries an explicit anycli.side_effect annotation with the reviewed value
// (design 318 may-mutate criterion), and that group commands carry none.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"forms get":               "false", // GET /forms/{id}
		"forms create":            "true",  // POST /forms
		"forms batch-update":      "true",  // POST /forms/{id}:batchUpdate
		"forms publish":           "true",  // POST /forms/{id}:setPublishSettings
		"forms unpublish":         "true",  // POST /forms/{id}:setPublishSettings
		"forms close":             "true",  // POST /forms/{id}:setPublishSettings
		"forms reopen":            "true",  // POST /forms/{id}:setPublishSettings
		"forms responses list":    "false", // GET /forms/{id}/responses
		"forms responses get":     "false", // GET /forms/{id}/responses/{rid}
		"forms responders list":   "false", // GET drive permissions
		"forms responders add":    "true",  // POST drive permissions
		"forms responders remove": "true",  // DELETE drive permissions/{pid}
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
