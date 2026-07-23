package delighted

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const testKey = "delk-test-123"

// capturedRequest records what the fake Delighted server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Accept string
	Auth   string
	Body   []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Accept: r.Header.Get("Accept"),
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: testKey})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, raw)
	}
	return m
}

// wantBasicAuth asserts the Authorization header is Basic base64(key:) — the
// key as username with an empty password.
func wantBasicAuth(t *testing.T, header string) {
	t.Helper()
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		t.Fatalf("Authorization = %q, want Basic scheme", header)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		t.Fatalf("Authorization not base64: %v", err)
	}
	if got, want := string(decoded), testKey+":"; got != want {
		t.Fatalf("Basic credentials = %q, want %q (key as username, blank password)", got, want)
	}
}

func TestMissingKeyFailsFast(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"metrics", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "DELIGHTED_API_KEY is not set") {
		t.Fatalf("stderr = %q, want missing-key message", errBuf.String())
	}
}

func TestMetricsGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"nps":42}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "metrics", "get", "--since", "1000", "--until", "2000", "--trend", "abc")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/metrics.json" {
		t.Fatalf("request = %s %s, want GET /metrics.json", got.Method, got.Path)
	}
	wantBasicAuth(t, got.Auth)
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got.Accept)
	}
	q := parseQuery(t, got.Query)
	if q.Get("since") != "1000" || q.Get("until") != "2000" || q.Get("trend") != "abc" {
		t.Fatalf("query = %v, want since/until/trend set", q)
	}
	if strings.TrimSpace(stdout) != `{"nps":42}` {
		t.Fatalf("stdout = %q, want verbatim body", stdout)
	}
}

func TestResponseListQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "response", "list",
		"--per-page", "50", "--page", "2", "--since", "10", "--until", "20",
		"--updated-since", "15", "--order", "desc", "--expand", "person")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Path != "/survey_responses.json" {
		t.Fatalf("path = %s, want /survey_responses.json", got.Path)
	}
	q := parseQuery(t, got.Query)
	for k, want := range map[string]string{
		"per_page": "50", "page": "2", "since": "10", "until": "20",
		"updated_since": "15", "order": "desc", "expand[]": "person",
	} {
		if q.Get(k) != want {
			t.Fatalf("query[%s] = %q, want %q (%v)", k, q.Get(k), want, q)
		}
	}
}

func TestResponseGetPathAndExpand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"r1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "response", "get", "--id", "r1", "--expand", "person")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/survey_responses/r1.json" {
		t.Fatalf("path = %s, want /survey_responses/r1.json", got.Path)
	}
	if parseQuery(t, got.Query).Get("expand[]") != "person" {
		t.Fatalf("query = %s, want expand[]=person", got.Query)
	}
}

func TestResponseCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":"r2"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "response", "create",
		"--person", "p1", "--score", "9", "--comment", "great",
		"--properties-json", `{"plan":"pro"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/survey_responses.json" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["person"] != "p1" || body["score"] != "9" || body["comment"] != "great" {
		t.Fatalf("body = %v, want person/score/comment", body)
	}
	props, ok := body["person_properties"].(map[string]any)
	if !ok || props["plan"] != "pro" {
		t.Fatalf("person_properties = %v, want {plan:pro}", body["person_properties"])
	}
}

func TestResponseUpdateOnlySendsChangedFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"r3"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "response", "update", "--id", "r3", "--tags", "vip,churn-risk")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodPut || got.Path != "/survey_responses/r3.json" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "vip" || tags[1] != "churn-risk" {
		t.Fatalf("tags = %v, want [vip churn-risk]", body["tags"])
	}
	if _, present := body["notes"]; present {
		t.Fatalf("notes should be omitted when not passed, body = %v", body)
	}
}

func TestPeopleSendBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"pp1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "people", "send",
		"--email", "a@b.com", "--name", "Ada", "--send=false",
		"--channel", "email", "--properties-json", `{"tier":"gold"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/people.json" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@b.com" || body["name"] != "Ada" || body["send"] != false || body["channel"] != "email" {
		t.Fatalf("body = %v", body)
	}
}

func TestPeopleDeleteAndCancelPending(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"ok":true}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "people", "delete", "--id", "p9"); exit != 0 {
		t.Fatalf("delete exit = %d", exit)
	}
	if got.Method != http.MethodDelete || got.Path != "/people/p9.json" {
		t.Fatalf("delete request = %s %s", got.Method, got.Path)
	}

	if exit, _, _ := run(t, srv, "people", "cancel-pending", "--email", "a@b.com"); exit != 0 {
		t.Fatalf("cancel exit = %d", exit)
	}
	if got.Method != http.MethodDelete || got.Path != "/people/a@b.com/survey_requests/pending.json" {
		t.Fatalf("cancel request = %s %s", got.Method, got.Path)
	}
}

func TestSuppressionEndpoints(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "bounces", "list", "--per-page", "10"); exit != 0 {
		t.Fatalf("bounces exit = %d", exit)
	}
	if got.Path != "/bounces.json" || parseQuery(t, got.Query).Get("per_page") != "10" {
		t.Fatalf("bounces request path/query = %s ? %s", got.Path, got.Query)
	}

	if exit, _, _ := run(t, srv, "unsubscribes", "list"); exit != 0 {
		t.Fatalf("unsub list exit = %d", exit)
	}
	if got.Path != "/unsubscribes.json" {
		t.Fatalf("unsub list path = %s", got.Path)
	}

	if exit, _, _ := run(t, srv, "unsubscribes", "add", "--person-email", "a@b.com"); exit != 0 {
		t.Fatalf("unsub add exit = %d", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/unsubscribes.json" {
		t.Fatalf("unsub add request = %s %s", got.Method, got.Path)
	}
	if decodeBody(t, got.Body)["person_email"] != "a@b.com" {
		t.Fatalf("unsub add body = %s", got.Body)
	}
}

func TestAutopilotMembershipPathsByPlatform(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"ok":true}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "autopilot", "memberships", "list", "--platform", "sms", "--person-email", "a@b.com"); exit != 0 {
		t.Fatalf("list exit = %d", exit)
	}
	if got.Path != "/autopilot/sms/memberships.json" {
		t.Fatalf("list path = %s", got.Path)
	}

	if exit, _, _ := run(t, srv, "autopilot", "memberships", "add", "--platform", "email", "--person-email", "a@b.com", "--name", "Ada"); exit != 0 {
		t.Fatalf("add exit = %d", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/autopilot/email/memberships.json" {
		t.Fatalf("add request = %s %s", got.Method, got.Path)
	}
	person, ok := decodeBody(t, got.Body)["person"].(map[string]any)
	if !ok || person["email"] != "a@b.com" || person["name"] != "Ada" {
		t.Fatalf("add body person = %v", decodeBody(t, got.Body))
	}

	if exit, _, _ := run(t, srv, "autopilot", "memberships", "remove", "--platform", "email", "--person-email", "a@b.com"); exit != 0 {
		t.Fatalf("remove exit = %d", exit)
	}
	if got.Method != http.MethodDelete || got.Path != "/autopilot/email/memberships.json" {
		t.Fatalf("remove request = %s %s", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("person_email") != "a@b.com" {
		t.Fatalf("remove query = %s", got.Query)
	}

	if exit, _, _ := run(t, srv, "autopilot", "config", "get", "--platform", "email"); exit != 0 {
		t.Fatalf("config exit = %d", exit)
	}
	if got.Path != "/autopilot/email.json" {
		t.Fatalf("config path = %s", got.Path)
	}
}

func TestAutopilotRejectsUnknownPlatform(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "autopilot", "config", "get", "--platform", "push")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 for bad platform", exit)
	}
	if !strings.Contains(stderr, "--platform must be email or sms") {
		t.Fatalf("stderr = %q, want platform validation message", stderr)
	}
	if got.Path != "" {
		t.Fatalf("no request should be made for a bad platform, got %s", got.Path)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"message":"Invalid API key"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "metrics", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("CredentialRejected = false, want true on 401")
	}
	if !strings.Contains(stderr, "Invalid API key") {
		t.Fatalf("stderr = %q, want provider message", stderr)
	}
}

func TestServerErrorDoesNotRejectCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 500, `{"message":"boom"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "metrics", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatalf("CredentialRejected = true, want false on 500")
	}
	if !strings.Contains(stderr, "boom") {
		t.Fatalf("stderr = %q, want provider message", stderr)
	}
}

func TestInvalidJSONFlagIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "response", "create", "--person", "p1", "--score", "9", "--properties-json", "{not-json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Fatalf("stderr = %q, want JSON validation message", stderr)
	}
	if got.Path != "" {
		t.Fatalf("no request should be made on a bad --properties-json, got %s", got.Path)
	}
}
