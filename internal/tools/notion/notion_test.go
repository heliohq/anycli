package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Notion server saw.
type capturedRequest struct {
	Method  string
	Path    string
	Auth    string
	Version string
	Query   url.Values
	Body    []byte
}

// newServer returns an httptest server answering every call with status +
// response, recording the last request into got.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Auth:    r.Header.Get("Authorization"),
			Version: r.Header.Get("Notion-Version"),
			Query:   r.URL.Query(),
			Body:    body,
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
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "secret-notion-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest, wantVersion string) {
	t.Helper()
	if got.Auth != "Bearer secret-notion-token" {
		t.Errorf("Authorization = %q, want Bearer secret-notion-token", got.Auth)
	}
	if got.Version != wantVersion {
		t.Errorf("Notion-Version = %q, want %s", got.Version, wantVersion)
	}
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"search", "--query", "x"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "NOTION_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		providerCode string
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, providerCode: "unauthorized", wantRejected: true},
		{name: "explicit unauthorized code", status: http.StatusBadRequest, providerCode: "unauthorized", wantRejected: true},
		{name: "restricted resource", status: http.StatusForbidden, providerCode: "restricted_resource", wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, providerCode: "rate_limited", wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, providerCode: "internal_server_error", wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"code":"`+tc.providerCode+`","message":"provider message"}`, &got)
			defer srv.Close()

			result, _, _ := runResult(t, srv, "search", "--query", "x")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestUnknownSubcommand_Fails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// Without NoArgs on the group commands, cobra silently prints help and
	// exits 0 for an unknown subcommand — a false success for an agent.
	code, _, stderr := run(t, srv, "page", "destroy", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for an unknown subcommand", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for an unknown subcommand, got %s", got.Path)
	}
}

func TestPageCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "create", "--parent", "parent-1", "--title", "Hello", "--content", "First paragraph")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/pages" {
		t.Errorf("request = %s %s, want POST /pages", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	parent, _ := payload["parent"].(map[string]any)
	if parent["page_id"] != "parent-1" {
		t.Errorf("parent = %v, want page_id parent-1", payload["parent"])
	}
	if _, ok := payload["children"]; !ok {
		t.Error("expected children block for --content")
	}
	if !strings.Contains(stdout, `"id":"p1"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestPageCreate_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"object":"error","code":"validation_error","message":"parent not found"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "create", "--parent", "bad", "--title", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "validation_error") || !strings.Contains(stderr, "parent not found") {
		t.Errorf("stderr = %q, want the Notion code and message", stderr)
	}
	// The 403/404 access hint must not leak onto other statuses (400 here).
	if strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, must not carry the 403/404 access hint on a 400", stderr)
	}
}

func TestPageGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p2"}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "page", "get", "p2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/pages/p2" {
		t.Errorf("request = %s %s, want GET /pages/p2", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	if !strings.Contains(stdout, `"id":"p2"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty when the page has no children", stderr)
	}
}

func TestPageGet_HasChildrenNudge(t *testing.T) {
	var got capturedRequest
	response := `{"object":"page","id":"p2","has_children":true}`
	srv := newServer(t, http.StatusOK, response, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "page", "get", "p2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout != response+"\n" {
		t.Errorf("stdout = %q, want the provider JSON verbatim", stdout)
	}
	if !strings.Contains(stderr, "notion page read") {
		t.Errorf("stderr = %q, want a nudge to page read when has_children is true", stderr)
	}
}

func TestPageGet_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"object":"error","code":"object_not_found","message":"no such page"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "get", "missing")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "object_not_found") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
	if !strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, want the access hint", stderr)
	}
}

func TestPageRead_Happy(t *testing.T) {
	var got capturedRequest
	response := `{"object":"page_markdown","id":"p9","markdown":"# Title\nbody","truncated":false,"unknown_block_ids":[]}`
	srv := newServer(t, http.StatusOK, response, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "read", "p9", "--include-transcript")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/pages/p9/markdown" {
		t.Errorf("request = %s %s, want GET /pages/p9/markdown", got.Method, got.Path)
	}
	assertAuth(t, got, "2026-03-11")
	if got.Query.Get("include_transcript") != "true" {
		t.Errorf("include_transcript = %q, want true", got.Query.Get("include_transcript"))
	}
	// Exact match: truncated / unknown_block_ids are the agent's re-fetch
	// signal — any reshaping of the body must fail here.
	if stdout != response+"\n" {
		t.Errorf("stdout = %q, want the page_markdown JSON verbatim", stdout)
	}
}

func TestPageRead_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"object":"error","code":"object_not_found","message":"no such page"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "read", "missing")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "object_not_found") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
	if !strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, want the access hint", stderr)
	}
}

func TestPageRead_DefaultNoTranscript(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page_markdown","id":"p9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "read", "p9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/pages/p9/markdown" {
		t.Errorf("path = %q, want /pages/p9/markdown", got.Path)
	}
	// Key-existence check: a present-but-empty include_transcript= would pass
	// a Get()!="" comparison.
	if _, ok := got.Query["include_transcript"]; ok {
		t.Errorf("include_transcript sent as %q, want absent when the flag is off", got.Query.Get("include_transcript"))
	}
}

func TestPageAppend_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "append", "p3", "--content", "more text")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/blocks/p3/children" {
		t.Errorf("request = %s %s, want PATCH /blocks/p3/children", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	if !strings.Contains(string(got.Body), "more text") {
		t.Errorf("body = %s, want the paragraph content", got.Body)
	}
}

func TestPageAppend_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"object":"error","code":"restricted_resource","message":"no access"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "append", "p3", "--content", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "restricted_resource") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
	if !strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, want the access hint on a 403", stderr)
	}
}

func TestSearch_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "search", "--query", "roadmap")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/search" {
		t.Errorf("request = %s %s, want POST /search", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	if !strings.Contains(string(got.Body), `"query":"roadmap"`) {
		t.Errorf("body = %s, want the query", got.Body)
	}
	if !strings.Contains(stdout, `"results"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestSearch_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"object":"error","code":"unauthorized","message":"token invalid"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search", "--query", "x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "unauthorized") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
	// The 403/404 access hint must not leak onto other statuses (401 here).
	if strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, must not carry the 403/404 access hint on a 401", stderr)
	}
}

func TestDBQuery_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "db", "query", "db-1", "--filter-json", `{"property":"Done","checkbox":{"equals":true}}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/databases/db-1/query" {
		t.Errorf("request = %s %s, want POST /databases/db-1/query", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if _, ok := payload["filter"]; !ok {
		t.Errorf("payload = %v, want the filter passed through", payload)
	}
}

func TestDBQuery_InvalidFilterJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "db", "query", "db-1", "--filter-json", "{not json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want the JSON validation error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for invalid filter JSON, got %s", got.Path)
	}
}

func TestDBQuery_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"object":"error","code":"validation_error","message":"bad filter"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "db", "query", "db-1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "bad filter") {
		t.Errorf("stderr = %q, want the Notion message", stderr)
	}
}

func TestBlockChildren_Happy(t *testing.T) {
	var got capturedRequest
	response := `{"object":"list","results":[],"has_more":false,"next_cursor":null}`
	srv := newServer(t, http.StatusOK, response, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "block", "children", "b1", "--page-size", "50", "--start-cursor", "cur123")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/blocks/b1/children" {
		t.Errorf("request = %s %s, want GET /blocks/b1/children", got.Method, got.Path)
	}
	assertAuth(t, got, "2022-06-28")
	if got.Query.Get("page_size") != "50" {
		t.Errorf("page_size = %q, want 50", got.Query.Get("page_size"))
	}
	if got.Query.Get("start_cursor") != "cur123" {
		t.Errorf("start_cursor = %q, want cur123", got.Query.Get("start_cursor"))
	}
	// Exact match: has_more / next_cursor drive the agent's pagination — any
	// reshaping of the body must fail here.
	if stdout != response+"\n" {
		t.Errorf("stdout = %q, want the list JSON verbatim", stdout)
	}
}

func TestBlockChildren_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"object":"error","code":"restricted_resource","message":"no access"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "block", "children", "b1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "restricted_resource") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
	if !strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, want the access hint", stderr)
	}
}

func TestBlockChildren_DefaultQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "block", "children", "b1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("page_size") != "100" {
		t.Errorf("page_size = %q, want 100 (the default)", got.Query.Get("page_size"))
	}
	// Key-existence check: a present-but-empty start_cursor= (which Notion
	// rejects) would pass a Get()!="" comparison.
	if _, ok := got.Query["start_cursor"]; ok {
		t.Errorf("start_cursor sent as %q, want absent when the flag is unset", got.Query.Get("start_cursor"))
	}
}
