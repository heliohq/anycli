package brex

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

// capturedRequest records what the fake Brex server saw.
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
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "secret-brex-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer secret-brex-token" {
		t.Errorf("Authorization = %q, want Bearer secret-brex-token", got.Auth)
	}
}

// TestResourceRouting proves every leaf hits the correct GET path.
func TestResourceRouting(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantPath string
	}{
		{"account card", []string{"account", "card"}, "/v2/accounts/card"},
		{"account cash list", []string{"account", "cash"}, "/v2/accounts/cash"},
		{"account cash get", []string{"account", "cash", "acc123"}, "/v2/accounts/cash/acc123"},
		{"transaction card-primary", []string{"transaction", "card-primary"}, "/v2/transactions/card/primary"},
		{"transaction cash", []string{"transaction", "cash", "cash1"}, "/v2/transactions/cash/cash1"},
		{"expense list", []string{"expense", "list"}, "/v1/expenses"},
		{"expense card", []string{"expense", "card"}, "/v1/expenses/card"},
		{"expense get", []string{"expense", "get", "exp1"}, "/v1/expenses/exp1"},
		{"card list", []string{"card", "list"}, "/v2/cards"},
		{"card get", []string{"card", "get", "cd1"}, "/v2/cards/cd1"},
		{"user list", []string{"user", "list"}, "/v2/users"},
		{"user me", []string{"user", "me"}, "/v2/users/me"},
		{"user get", []string{"user", "get", "u1"}, "/v2/users/u1"},
		{"budget list", []string{"budget", "list"}, "/v2/budgets"},
		{"budget get", []string{"budget", "get", "b1"}, "/v2/budgets/b1"},
		{"budget spend-limits", []string{"budget", "spend-limits"}, "/v2/spend_limits"},
		{"department list", []string{"department", "list"}, "/v2/departments"},
		{"location list", []string{"location", "list"}, "/v2/locations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{"items":[],"next_cursor":null}`, &got)
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

// TestPaginationParams asserts --limit / --cursor are sent as query parameters.
func TestPaginationParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"next_cursor":null}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "card", "list", "--limit", "5", "--cursor", "abc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("limit") != "5" {
		t.Errorf("limit = %q, want 5", got.Query.Get("limit"))
	}
	if got.Query.Get("cursor") != "abc" {
		t.Errorf("cursor = %q, want abc", got.Query.Get("cursor"))
	}
}

// TestPaginationAll follows next_cursor and accumulates items into one envelope.
func TestPaginationAll(t *testing.T) {
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, capturedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "cur2" {
			_, _ = w.Write([]byte(`{"items":[{"id":"c2"}],"next_cursor":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"c1"}],"next_cursor":"cur2"}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"card", "list", "--all"}, map[string]string{EnvToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if len(reqs) != 2 {
		t.Fatalf("made %d requests, want 2 (--all must follow next_cursor)", len(reqs))
	}
	var merged struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(out.Bytes(), &merged); err != nil {
		t.Fatalf("merged output not JSON: %v", err)
	}
	if len(merged.Items) != 2 || merged.NextCursor != nil {
		t.Errorf("merged = %+v, want both items aggregated and next_cursor=null", merged)
	}
}

// TestFirstPageVerbatim: without --all, the first page envelope is returned
// verbatim (next_cursor intact for manual continuation).
func TestFirstPageVerbatim(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[{"id":"x"}],"next_cursor":"more"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "expense", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, `"next_cursor":"more"`) {
		t.Errorf("stdout = %q, want the raw envelope with next_cursor intact", stdout)
	}
}

// TestGetPassthrough: `get <path>` hits the raw path and forwards --param query.
func TestGetPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "get", "/v2/cards", "--param", "status=ACTIVE")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v2/cards" {
		t.Errorf("path = %q, want /v2/cards", got.Path)
	}
	if got.Query.Get("status") != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", got.Query.Get("status"))
	}
	assertAuth(t, got)
}

// TestGetPassthrough_AddsLeadingSlash: a path without a leading slash is
// normalized.
func TestGetPassthrough_AddsLeadingSlash(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "get", "v2/budgets")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v2/budgets" {
		t.Errorf("path = %q, want /v2/budgets", got.Path)
	}
}

// TestGetPassthrough_RejectsAbsoluteURL: an absolute URL is a usage error, no
// request is sent.
func TestGetPassthrough_RejectsAbsoluteURL(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "get", "https://evil.example.com/v2/cards")
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

// TestAPIError_Text: a non-2xx surfaces the Brex message as an exit-1 apiError
// in plain text.
func TestAPIError_Text(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"message":"invalid budget id"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "budget", "get", "bad")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid budget id") || !strings.Contains(stderr, "422") {
		t.Errorf("stderr = %q, want the Brex message and HTTP 422", stderr)
	}
}

// TestAPIError_JSON: under --json the error renders as the structured envelope
// with kind=api and the HTTP status.
func TestAPIError_JSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"bad request"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "card", "list", "--json")
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
		t.Errorf("error.message = %q, want the Brex message", env.Error.Message)
	}
}

// TestUnauthorized_RejectsCredential: a 401 marks the result as a credential
// rejection so the token gateway refreshes and retries (design 227).
func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"token expired"}`, &got)
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "me"}, map[string]string{EnvToken: "expired"})
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
	result, err := svc.Execute(context.Background(), []string{"user", "me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "BREX_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

// TestMissingToken_JSON: the pre-parse missing-token error honors --json.
func TestMissingToken_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "me", "--json"}, map[string]string{})
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
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "BREX_ACCESS_TOKEN") {
		t.Errorf("error = %+v, want kind=usage with the missing-token message", env.Error)
	}
}

// TestUsageError_MissingArg: a required id arg missing is exit 2, no request.
func TestUsageError_MissingArg(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "card", "get")
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

	code, _, stderr := run(t, srv, "card", "delete", "x")
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
	srv := newServer(t, http.StatusOK, `{"id":"u1","email":"a@x.com"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "get", "u1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, `"u1"`) || !strings.Contains(stdout, "a@x.com") {
		t.Errorf("stdout = %q, want the verbatim user object", stdout)
	}
}

// TestAcceptHeader confirms the Accept: application/json header is set.
func TestAcceptHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	_, _, _ = run(t, srv, "user", "me")
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}
