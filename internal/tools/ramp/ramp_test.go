package ramp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// capturedRequest records what the fake Ramp server saw.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Accept string
	Query  url.Values
}

// newServer returns an httptest server answering every call with status +
// response, recording the last request into got.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"),
			Query:  r.URL.Query(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "secret-ramp-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer secret-ramp-token" {
		t.Errorf("Authorization = %q, want Bearer secret-ramp-token", got.Auth)
	}
}

// TestResourceRouting proves every leaf hits the correct GET path.
func TestResourceRouting(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantPath string
	}{
		{"transaction list", []string{"transaction", "list"}, "/developer/v1/transactions"},
		{"transaction get", []string{"transaction", "get", "tx1"}, "/developer/v1/transactions/tx1"},
		{"reimbursement list", []string{"reimbursement", "list"}, "/developer/v1/reimbursements"},
		{"reimbursement get", []string{"reimbursement", "get", "rb1"}, "/developer/v1/reimbursements/rb1"},
		{"card virtual list", []string{"card", "virtual"}, "/developer/v1/cards/virtual"},
		{"card virtual get", []string{"card", "virtual", "cv1"}, "/developer/v1/cards/virtual/cv1"},
		{"card physical list", []string{"card", "physical"}, "/developer/v1/cards/physical"},
		{"card physical get", []string{"card", "physical", "cp1"}, "/developer/v1/cards/physical/cp1"},
		{"user list", []string{"user", "list"}, "/developer/v1/users"},
		{"user get", []string{"user", "get", "u1"}, "/developer/v1/users/u1"},
		{"department list", []string{"department", "list"}, "/developer/v1/departments"},
		{"department get", []string{"department", "get", "d1"}, "/developer/v1/departments/d1"},
		{"location list", []string{"location", "list"}, "/developer/v1/locations"},
		{"location get", []string{"location", "get", "l1"}, "/developer/v1/locations/l1"},
		{"business info", []string{"business", "info"}, "/developer/v1/business"},
		{"business balance", []string{"business", "balance"}, "/developer/v1/business/balance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{"data":[],"page":{"next":null}}`, &got)
			defer srv.Close()

			code, _, stderr := run(t, srv, tc.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
			}
			if got.Method != http.MethodGet || got.Path != tc.wantPath {
				t.Errorf("request = %s %s, want GET %s", got.Method, got.Path, tc.wantPath)
			}
			assertAuth(t, got)
		})
	}
}

// TestPaginationParams asserts --limit / --cursor map to Ramp's page_size / start
// wire query params.
func TestPaginationParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[],"page":{"next":null}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "transaction", "list", "--limit", "5", "--cursor", "abc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("page_size") != "5" {
		t.Errorf("page_size = %q, want 5", got.Query.Get("page_size"))
	}
	if got.Query.Get("start") != "abc" {
		t.Errorf("start = %q, want abc", got.Query.Get("start"))
	}
}

// TestPaginationAll follows page.next (a full URL) by extracting its start
// cursor, accumulating data into one envelope.
func TestPaginationAll(t *testing.T) {
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, capturedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start") == "cur2" {
			_, _ = w.Write([]byte(`{"data":[{"id":"c2"}],"page":{"next":null}}`))
			return
		}
		// page.next is a full URL carrying the next start cursor.
		_, _ = w.Write([]byte(`{"data":[{"id":"c1"}],"page":{"next":"https://api.ramp.com/developer/v1/transactions?start=cur2&page_size=1"}}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"transaction", "list", "--all"}, map[string]string{EnvToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", result.ExitCode, errBuf.String())
	}
	if len(reqs) != 2 {
		t.Fatalf("made %d requests, want 2 (--all must follow page.next)", len(reqs))
	}
	if reqs[1].Query.Get("start") != "cur2" {
		t.Errorf("second request start = %q, want cur2 (extracted from page.next URL)", reqs[1].Query.Get("start"))
	}
	var merged struct {
		Data []map[string]any `json:"data"`
		Page struct {
			Next *string `json:"next"`
		} `json:"page"`
	}
	if err := json.Unmarshal(out.Bytes(), &merged); err != nil {
		t.Fatalf("merged output not JSON: %v", err)
	}
	if len(merged.Data) != 2 || merged.Page.Next != nil {
		t.Errorf("merged = %+v, want both rows aggregated and page.next=null", merged)
	}
}

// TestFirstPageVerbatim: without --all, the first page envelope is returned
// verbatim (page.next intact for manual continuation).
func TestFirstPageVerbatim(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"x"}],"page":{"next":"https://api.ramp.com/developer/v1/transactions?start=more"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "transaction", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "start=more") {
		t.Errorf("stdout = %q, want the raw envelope with page.next intact", stdout)
	}
}

// TestGetPassthrough: `get <path>` hits the raw path and forwards --param query.
func TestGetPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "get", "/developer/v1/transactions", "--param", "state=CLEARED")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/developer/v1/transactions" {
		t.Errorf("path = %q, want /developer/v1/transactions", got.Path)
	}
	if got.Query.Get("state") != "CLEARED" {
		t.Errorf("state = %q, want CLEARED", got.Query.Get("state"))
	}
	assertAuth(t, got)
}

// TestGetPassthrough_AddsLeadingSlash: a path without a leading slash is
// normalized.
func TestGetPassthrough_AddsLeadingSlash(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "get", "developer/v1/users")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/developer/v1/users" {
		t.Errorf("path = %q, want /developer/v1/users", got.Path)
	}
}

// TestGetPassthrough_RejectsAbsoluteURL: an absolute URL is a usage error, no
// request is sent.
func TestGetPassthrough_RejectsAbsoluteURL(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "get", "https://evil.example.com/developer/v1/users")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an absolute URL", code)
	}
	if !strings.Contains(stderr, "must be relative") {
		t.Errorf("stderr = %q, want the relative-path guard", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestAPIError_Text: a non-2xx surfaces Ramp's nested error.message as an exit-1
// apiError in plain text with the HTTP status.
func TestAPIError_Text(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"error":{"message":"invalid transaction id"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "transaction", "get", "bad")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid transaction id") || !strings.Contains(stderr, "422") {
		t.Errorf("stderr = %q, want the Ramp message and HTTP 422", stderr)
	}
}

// TestAPIError_TopLevelMessage: a top-level {"message":...} error body is also
// surfaced (Ramp's shapes vary by endpoint).
func TestAPIError_TopLevelMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"bad filter"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "transaction", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "bad filter") {
		t.Errorf("stderr = %q, want the top-level message", stderr)
	}
}

// TestAPIError_JSON: under --json the error renders as the structured envelope
// with kind=api and the HTTP status.
func TestAPIError_JSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":{"message":"bad request"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "card", "virtual", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("error = %+v, want kind=api status=400", env.Error)
	}
	if !strings.Contains(env.Error.Message, "bad request") {
		t.Errorf("error.message = %q, want the Ramp message", env.Error.Message)
	}
}

// TestUnauthorized_RejectsCredential: a 401 marks the result as a credential
// rejection so the token gateway refreshes and retries (design 227).
func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":{"message":"token expired"}}`, &got)
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"business", "info"}, map[string]string{EnvToken: "expired"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for a 401")
	}
}

// TestMissingToken: no token → exit 1 with an explicit message (plain text).
func TestMissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"business", "info"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "RAMP_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

// TestMissingToken_JSON: the pre-parse missing-token error honors --json.
func TestMissingToken_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"business", "info", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr is not JSON: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "RAMP_ACCESS_TOKEN") {
		t.Errorf("error = %+v, want kind=usage with the missing-token message", env.Error)
	}
}

// TestUsageError_MissingArg: a required id arg missing is exit 2, no request.
func TestUsageError_MissingArg(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing id", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestUsageError_UnknownSubcommand: an unknown subcommand is exit 2.
func TestUsageError_UnknownSubcommand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "user", "delete", "x")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for unknown subcommand", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want unknown command", stderr)
	}
}

// TestSingleObjectVerbatim: a single-object GET emits the body verbatim.
func TestSingleObjectVerbatim(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"biz1","business_name_legal":"Acme Inc"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "business", "info")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, `"biz1"`) || !strings.Contains(stdout, "Acme Inc") {
		t.Errorf("stdout = %q, want the verbatim business object", stdout)
	}
}

// TestAcceptHeader confirms the Accept: application/json header is set.
func TestAcceptHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	_, _, _ = run(t, srv, "business", "info")
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}
