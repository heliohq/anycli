package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records what the fake Notion server saw.
type capturedRequest struct {
	Method  string
	Path    string
	Auth    string
	Version string
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
			Body:    body,
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
	code, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "secret-notion-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return code, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer secret-notion-token" {
		t.Errorf("Authorization = %q, want Bearer secret-notion-token", got.Auth)
	}
	if got.Version != "2022-06-28" {
		t.Errorf("Notion-Version = %q, want 2022-06-28", got.Version)
	}
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	code, err := svc.Execute(context.Background(), []string{"search", "--query", "x"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "NOTION_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
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
	assertAuth(t, got)
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
}

func TestPageGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p2"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "get", "p2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/pages/p2" {
		t.Errorf("request = %s %s, want GET /pages/p2", got.Method, got.Path)
	}
	assertAuth(t, got)
	if !strings.Contains(stdout, `"id":"p2"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
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
	assertAuth(t, got)
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
	assertAuth(t, got)
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
	assertAuth(t, got)
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
