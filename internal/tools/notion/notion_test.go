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

func TestExecute_MissingToken_JSON(t *testing.T) {
	// The missing-token check runs before cobra parses flags, but --json in the
	// raw args must still yield the structured error envelope on stderr (§error).
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"search", "--query", "x", "--json"}, map[string]string{})
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
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "NOTION_TOKEN is not set") {
		t.Errorf("envelope = %+v, want kind=usage with the missing-token message", env.Error)
	}
}

func TestSearch_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[],"has_more":false,"next_cursor":null}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "search", "--query", "roadmap")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/search" {
		t.Errorf("request = %s %s, want POST /search", got.Method, got.Path)
	}
	// search runs on the 2026-03-11 data model (data_source object filter).
	assertAuth(t, got, markdownVersion)
	if !strings.Contains(string(got.Body), `"query":"roadmap"`) {
		t.Errorf("body = %s, want the query", got.Body)
	}
	if !strings.Contains(stdout, `"results"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "--query", "x", "--type", "data_source")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	filter, _ := payload["filter"].(map[string]any)
	if filter["property"] != "object" || filter["value"] != "data_source" {
		t.Errorf("filter = %v, want object=data_source", payload["filter"])
	}
}

func TestSearch_BadType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search", "--query", "x", "--type", "database")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a bad enum", code)
	}
	if !strings.Contains(stderr, "must be one of") {
		t.Errorf("stderr = %q, want the enum validation error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for a bad enum, got %s", got.Path)
	}
}

func TestSearch_MissingQuery_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a missing required flag", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent when --query is missing, got %s", got.Path)
	}
}

func TestSearch_All_Paginates(t *testing.T) {
	// Two-page result: the first page has_more with a cursor, the second ends
	// it. --all must follow the cursor and merge results.
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, capturedRequest{Body: body})
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		w.Header().Set("Content-Type", "application/json")
		if payload["start_cursor"] == "cur2" {
			_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"b"}],"has_more":false,"next_cursor":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"a"}],"has_more":true,"next_cursor":"cur2"}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"search", "--query", "x", "--all"}, map[string]string{EnvToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if len(reqs) != 2 {
		t.Fatalf("made %d requests, want 2 (followed the cursor)", len(reqs))
	}
	var merged struct {
		Results []map[string]any `json:"results"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(out.String()), &merged); err != nil {
		t.Fatalf("merged output not JSON: %v", err)
	}
	if len(merged.Results) != 2 || merged.HasMore {
		t.Errorf("merged = %+v, want 2 results and has_more=false", merged)
	}
}

func TestSearch_APIError_CredentialRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"object":"error","code":"unauthorized","message":"token invalid"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "search", "--query", "x")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 for an API error", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true on a 401 unauthorized")
	}
	if !strings.Contains(stderr, "unauthorized") {
		t.Errorf("stderr = %q, want the Notion error code", stderr)
	}
}

func TestSearch_APIError_NotRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"object":"error","code":"restricted_resource","message":"no access"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "search", "--query", "x")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false on a 403 restricted_resource")
	}
	if !strings.Contains(stderr, "check the ID and that the integration has been granted access") {
		t.Errorf("stderr = %q, want the 403 access hint", stderr)
	}
}

func TestSearch_JSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"object":"error","code":"validation_error","message":"bad"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search", "--query", "x", "--json")
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
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusBadRequest {
		t.Errorf("envelope = %+v, want kind=api status=400", env.Error)
	}
	if !strings.Contains(env.Error.Message, "validation_error") {
		t.Errorf("message = %q, want the Notion code", env.Error.Message)
	}
}

func TestFetch_Page_Markdown(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page_markdown","id":"p9","markdown":"# Title\nbody","truncated":false,"unknown_block_ids":[]}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "fetch", "p9", "--type", "page")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/pages/p9/markdown" {
		t.Errorf("request = %s %s, want GET /pages/p9/markdown", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	if stdout != "# Title\nbody\n" {
		t.Errorf("stdout = %q, want the bare markdown", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty for a complete read", stderr)
	}
}

func TestFetch_Page_PartialNote(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"partial","truncated":true,"unknown_block_ids":["b1"]}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "fetch", "p9", "--type", "page")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout != "partial\n" {
		t.Errorf("stdout = %q, want the bare markdown only", stdout)
	}
	if !strings.Contains(stderr, "partial") {
		t.Errorf("stderr = %q, want the re-fetch note", stderr)
	}
}

func TestFetch_Page_JSONForcesEnvelope(t *testing.T) {
	var got capturedRequest
	response := `{"object":"page_markdown","id":"p9","markdown":"x"}`
	srv := newServer(t, http.StatusOK, response, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "fetch", "p9", "--type", "page", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout != response+"\n" {
		t.Errorf("stdout = %q, want the full envelope verbatim under --json", stdout)
	}
}

func TestFetch_DataSource_JSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"data_source","id":"ds1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "fetch", "ds1", "--type", "data_source")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/data_sources/ds1" {
		t.Errorf("request = %s %s, want GET /data_sources/ds1", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	if !strings.Contains(stdout, `"id":"ds1"`) {
		t.Errorf("stdout = %q, want the provider JSON", stdout)
	}
}

func TestFetch_Database_JSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"database","id":"db1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "fetch", "db1", "--type", "database")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/databases/db1" {
		t.Errorf("path = %q, want /databases/db1", got.Path)
	}
}

func TestFetch_Probe_PageWins(t *testing.T) {
	// newServer answers 200 to everything, so the first probe (page markdown)
	// wins and the id types as a page.
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page_markdown","markdown":"hi"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "fetch", "p9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/pages/p9/markdown" {
		t.Errorf("path = %q, want the page markdown endpoint via probe", got.Path)
	}
	if stdout != "hi\n" {
		t.Errorf("stdout = %q, want the probed page markdown", stdout)
	}
}

func TestFetch_Self_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "fetch", "self")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for fetch self", code)
	}
	if !strings.Contains(stderr, "user get self") {
		t.Errorf("stderr = %q, want a redirect to user get self", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for fetch self, got %s", got.Path)
	}
}

func TestUnknownSubcommand_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// A runnable group rejects an unknown subcommand instead of exiting 0 with
	// help — a false success for an agent. It is a usage error → exit 2.
	code, _, stderr := run(t, srv, "page", "destroy", "x")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an unknown subcommand", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for an unknown subcommand, got %s", got.Path)
	}
}

func TestResolveID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc", "abc"},
		{"12345678123412341234123456789012", "12345678-1234-1234-1234-123456789012"},
		{"12345678-1234-1234-1234-123456789012", "12345678-1234-1234-1234-123456789012"},
		{"https://www.notion.so/Team-Page-12345678123412341234123456789012", "12345678-1234-1234-1234-123456789012"},
		{"https://www.notion.so/12345678123412341234123456789012?v=aaaaaaaabbbbccccddddeeeeffff0000", "12345678-1234-1234-1234-123456789012"},
	}
	for _, tc := range cases {
		got, err := resolveID(tc.in)
		if err != nil {
			t.Errorf("resolveID(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIconWire(t *testing.T) {
	emoji, err := iconWire("🚀")
	if err != nil {
		t.Fatalf("emoji icon: %v", err)
	}
	if !strings.Contains(string(emoji), `"emoji":"🚀"`) {
		t.Errorf("emoji wire = %s, want an emoji object", emoji)
	}
	ext, err := iconWire("https://x/y.png")
	if err != nil {
		t.Fatalf("url icon: %v", err)
	}
	if !strings.Contains(string(ext), `"external"`) {
		t.Errorf("url wire = %s, want an external object", ext)
	}
	if _, err := iconWire("plain"); err == nil {
		t.Error("iconWire(plain) = nil error, want a usage error")
	}
	if _, err := coverWire("nope"); err == nil {
		t.Error("coverWire(nope) = nil error, want a usage error")
	}
}
