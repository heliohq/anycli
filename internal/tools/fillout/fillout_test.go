package fillout

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Fillout server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

func newServer(t *testing.T, response string, got *capturedRequest) *httptest.Server {
	return newServerWithStatus(t, http.StatusOK, response, got)
}

func newServerWithStatus(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// run executes the service against srv with the api base injected via the
// Service.APIBase override, capturing output.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runResult(t, srv, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{APIBase: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "flo_test_token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"form", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "FILLOUT_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestResolveBase(t *testing.T) {
	if got := resolveBase(""); got != DefaultAPIBase {
		t.Errorf("resolveBase(\"\") = %q, want default %q", got, DefaultAPIBase)
	}
	const eu = "https://eu-api.fillout.com"
	if got := resolveBase(eu); got != eu {
		t.Errorf("resolveBase(EU) = %q, want passthrough %q", got, eu)
	}
}

// TestAPIBaseFromEnv proves the FILLOUT_API_BASE env value is honored when the
// Service.APIBase override is empty (the production injection path).
func TestAPIBaseFromEnv(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"forms":[]}`, &got)
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"form", "list"},
		map[string]string{EnvAccessToken: "flo_test_token", EnvAPIBase: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", result.ExitCode, errBuf.String())
	}
	if got.Path != "/v1/api/forms" {
		t.Errorf("path = %q, want /v1/api/forms", got.Path)
	}
}

func TestFormList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"forms":[{"formId":"abc","name":"Intake"}]}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "form", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/api/forms" {
		t.Errorf("request = %s %s, want GET /v1/api/forms", got.Method, got.Path)
	}
	if got.Auth != "Bearer flo_test_token" {
		t.Errorf("Authorization = %q, want Bearer flo_test_token", got.Auth)
	}
	if !strings.Contains(stdout, `"Intake"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestFormGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"id":"F1","name":"Survey","questions":[]}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "form", "get", "F1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/api/forms/F1" {
		t.Errorf("request = %s %s, want GET /v1/api/forms/F1", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"questions"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestFormGet_MissingArg(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "form", "get")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing positional arg", code)
	}
	if stderr == "" {
		t.Error("expected an arg error on stderr")
	}
	if got.Path != "" {
		t.Errorf("no request must be sent on arg errors, got %s", got.Path)
	}
}

func TestSubmissionList_Happy_WithFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"responses":[],"totalResponses":0}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "submission", "list", "F1",
		"--limit", "25", "--offset", "10", "--status", "in_progress",
		"--after-date", "2024-01-01T00:00:00Z", "--before-date", "2024-12-31T00:00:00Z",
		"--sort", "desc", "--search", "acme", "--include-edit-link", "--include-preview")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/api/forms/F1/submissions" {
		t.Errorf("request = %s %s, want GET /v1/api/forms/F1/submissions", got.Method, got.Path)
	}
	for _, want := range []string{
		"limit=25", "offset=10", "status=in_progress",
		"afterDate=2024-01-01T00%3A00%3A00Z", "beforeDate=2024-12-31T00%3A00%3A00Z",
		"sort=desc", "search=acme", "includeEditLink=true", "includePreview=true",
	} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want it to contain %q", got.Query, want)
		}
	}
	if !strings.Contains(stdout, `"responses"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestSubmissionList_NoFlags_NoQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"responses":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "list", "F1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty when no flags are set", got.Query)
	}
}

func TestSubmissionList_InvalidStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "list", "F1", "--status", "bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid enum", code)
	}
	if !strings.Contains(stderr, "status") {
		t.Errorf("stderr = %q, want a status validation error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent on validation errors, got %s", got.Path)
	}
}

func TestSubmissionGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"submission":{"submissionId":"S1"}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "submission", "get", "F1", "S1", "--include-edit-link")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/api/forms/F1/submissions/S1" {
		t.Errorf("request = %s %s, want GET /v1/api/forms/F1/submissions/S1", got.Method, got.Path)
	}
	if !strings.Contains(got.Query, "includeEditLink=true") {
		t.Errorf("query = %q, want includeEditLink=true", got.Query)
	}
	if !strings.Contains(stdout, `"submissionId"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestSubmissionCreate_Data(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"submissions":[{"submissionId":"S9"}]}`, &got)
	defer srv.Close()

	payload := `{"submissions":[{"questions":[{"id":"q1","value":"hi"}]}]}`
	code, stdout, stderr := run(t, srv, "submission", "create", "F1", "--data", payload)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/api/forms/F1/submissions" {
		t.Errorf("request = %s %s, want POST /v1/api/forms/F1/submissions", got.Method, got.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if _, ok := sent["submissions"]; !ok {
		t.Errorf("body = %s, want a submissions array", string(got.Body))
	}
	if !strings.Contains(stdout, `"S9"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestSubmissionCreate_InvalidJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "create", "F1", "--data", "{not json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid JSON body", code)
	}
	if stderr == "" {
		t.Error("expected a JSON validation error on stderr")
	}
	if got.Path != "" {
		t.Errorf("no request must be sent on validation errors, got %s", got.Path)
	}
}

func TestSubmissionCreate_MissingBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "create", "F1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when neither --data nor --file is given", code)
	}
	if stderr == "" {
		t.Error("expected a usage error on stderr")
	}
}

func TestSubmissionDelete_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "delete", "F1", "S1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/api/forms/F1/submissions/S1" {
		t.Errorf("request = %s %s, want DELETE /v1/api/forms/F1/submissions/S1", got.Method, got.Path)
	}
}

func TestWebhookCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"id":123}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "webhook", "create", "--form-id", "F1", "--url", "https://example.com/hook")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/api/webhook/create" {
		t.Errorf("request = %s %s, want POST /v1/api/webhook/create", got.Method, got.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if sent["formId"] != "F1" || sent["url"] != "https://example.com/hook" {
		t.Errorf("body = %s, want formId + url set", string(got.Body))
	}
	if !strings.Contains(stdout, `123`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestWebhookDelete_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, `{"success":true}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "webhook", "delete", "--webhook-id", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/api/webhook/delete" {
		t.Errorf("request = %s %s, want POST /v1/api/webhook/delete", got.Method, got.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if sent["webhookId"] != "123" {
		t.Errorf("body = %s, want webhookId set", string(got.Body))
	}
}

func TestAPIError_PlainAndJSON(t *testing.T) {
	var got capturedRequest
	srv := newServerWithStatus(t, http.StatusBadRequest, `{"message":"bad form id"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "form", "get", "F1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for an API error", code)
	}
	if !strings.Contains(stderr, "bad form id") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}

	var got2 capturedRequest
	srv2 := newServerWithStatus(t, http.StatusBadRequest, `{"message":"bad form id"}`, &got2)
	defer srv2.Close()
	code, _, stderr = run(t, srv2, "form", "get", "F1", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for an API error (--json)", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (stderr=%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusBadRequest {
		t.Errorf("envelope = %+v, want kind=api status=400", env.Error)
	}
}

func TestCredentialRejection(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "server error", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServerWithStatus(t, tc.status, `{"message":"nope"}`, &got)
			defer srv.Close()

			result, _, _ := runResult(t, srv, "form", "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}
