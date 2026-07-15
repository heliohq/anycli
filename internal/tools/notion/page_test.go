package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageCreate_Happy(t *testing.T) {
	var got capturedRequest
	// POST /v1/pages creates a single page and returns that page object; there
	// is no batch envelope, so the CLI fans out one request per --pages element.
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"np1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"par"},"properties":{"title":"Hello"},"content":"# Hi"}]`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/pages" {
		t.Errorf("request = %s %s, want POST /pages", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	// The element's fields are spread at the top level (parent required by the
	// endpoint), NOT wrapped in a "pages" batch envelope.
	body := bodyMap(t, got.Body)
	if _, ok := body["pages"]; ok {
		t.Errorf("body must not carry a pages batch envelope, got %v", body["pages"])
	}
	parent, ok := body["parent"].(map[string]any)
	if !ok || parent["page_id"] != "par" {
		t.Errorf("body.parent = %v, want the spread {page_id:par}", body["parent"])
	}
	// A string `content` (MCP markdown) is mapped to REST `markdown`; `content`
	// must not survive (REST `content` is a block array, not a markdown string).
	if body["markdown"] != "# Hi" {
		t.Errorf("body.markdown = %v, want the content mapped to markdown", body["markdown"])
	}
	if _, ok := body["content"]; ok {
		t.Errorf("body.content must be renamed to markdown, got %v", body["content"])
	}
	if strings.TrimSpace(stdout) != "np1" {
		t.Errorf("stdout = %q, want the created page id", stdout)
	}
}

// TestPageCreate_MultipleFanOut: several --pages elements each become one
// POST /v1/pages, and every created id is listed.
func TestPageCreate_MultipleFanOut(t *testing.T) {
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		w.Header().Set("Content-Type", "application/json")
		// String `content` is mapped to `markdown` before the request is sent.
		title, _ := p["markdown"].(string)
		w.Write([]byte(`{"object":"page","id":"id-` + title + `"}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"page", "create",
		"--pages", `[{"parent":{"page_id":"a"},"content":"one"},{"parent":{"page_id":"b"},"content":"two"}]`},
		map[string]string{EnvToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if countReq(reqs, http.MethodPost, "/pages") != 2 {
		t.Fatalf("made %d POST /pages, want 2 (one per element)", countReq(reqs, http.MethodPost, "/pages"))
	}
	lines := strings.Fields(out.String())
	if len(lines) != 2 || lines[0] != "id-one" || lines[1] != "id-two" {
		t.Errorf("stdout ids = %v, want [id-one id-two]", lines)
	}
}

func TestPageCreate_JSONReturnsFull(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"np1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"}}]`, "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// --json aggregates the per-page response bodies under a "pages" array.
	var env struct {
		Pages []map[string]any `json:"pages"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("stdout not a JSON pages envelope: %v (%q)", err, stdout)
	}
	if len(env.Pages) != 1 || env.Pages[0]["id"] != "np1" {
		t.Errorf("pages = %v, want a one-element array carrying the created page", env.Pages)
	}
}

func TestPageCreate_EmptyArray_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "create", "--pages", `[]`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an empty --pages array", code)
	}
	if !strings.Contains(stderr, "at least one page object") {
		t.Errorf("stderr = %q, want the empty-array usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for an empty --pages, got %s", got.Path)
	}
}

func TestPageCreate_BadJSON_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "create", "--pages", `{not json`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid JSON", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want an invalid-JSON usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for invalid --pages, got %s", got.Path)
	}
}

func TestPageUpdate_ReplaceContent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"# updated"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "update", "p1", "--command", "replace_content", "--new-str", "# updated")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/pages/p1/markdown" {
		t.Errorf("request = %s %s, want PATCH /pages/p1/markdown", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	body := bodyMap(t, got.Body)
	// CLI --command maps to the REST field `type`; the op params nest under an
	// object keyed by that type value (the markdown endpoint rejects a flat body).
	if body["type"] != "replace_content" {
		t.Errorf("body.type = %v, want replace_content", body["type"])
	}
	op, ok := body["replace_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.replace_content = %v, want a nested op object", body["replace_content"])
	}
	if op["new_str"] != "# updated" {
		t.Errorf("replace_content.new_str = %v, want the replacement markdown", op["new_str"])
	}
	if _, ok := body["new_str"]; ok {
		t.Errorf("new_str must not be at the top level, got %v", body["new_str"])
	}
	if stdout != "# updated\n" {
		t.Errorf("stdout = %q, want the post-update markdown", stdout)
	}
}

func TestPageUpdate_InsertContent_DefaultEnd(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"body"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "update", "p1", "--command", "insert_content", "--content", "more")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	op, ok := body["insert_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.insert_content = %v, want a nested op object", body["insert_content"])
	}
	if body["type"] != "insert_content" || op["content"] != "more" {
		t.Errorf("body = %v, want insert_content/content", body)
	}
	// --position is always sent, defaulting to end (never omitted).
	pos, ok := op["position"].(map[string]any)
	if !ok || pos["type"] != "end" {
		t.Errorf("insert_content.position = %v, want {type:end} sent explicitly", op["position"])
	}
}

func TestPageUpdate_UpdateContent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "update", "p1", "--command", "update_content",
		"--content-updates", `[{"old_str":"foo","new_str":"bar","replace_all_matches":true}]`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if body["type"] != "update_content" {
		t.Errorf("body.type = %v, want update_content", body["type"])
	}
	op, ok := body["update_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.update_content = %v, want a nested op object", body["update_content"])
	}
	ups, ok := op["content_updates"].([]any)
	if !ok || len(ups) != 1 {
		t.Fatalf("update_content.content_updates = %v, want a one-element array", op["content_updates"])
	}
	first := ups[0].(map[string]any)
	if first["old_str"] != "foo" || first["new_str"] != "bar" || first["replace_all_matches"] != true {
		t.Errorf("content_updates[0] = %v, want the passthrough item", first)
	}
}

// TestPageUpdate_Matrix exercises the design-304 §④ fail-fast matrix: illegal
// flag/command combinations are rejected before any request (exit 2).
func TestPageUpdate_Matrix(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"replace forbids content-updates", []string{"page", "update", "p1", "--command", "replace_content", "--new-str", "x", "--content-updates", "[]"}},
		{"replace requires new-str", []string{"page", "update", "p1", "--command", "replace_content"}},
		{"update requires content-updates", []string{"page", "update", "p1", "--command", "update_content"}},
		{"insert forbids allow-deleting", []string{"page", "update", "p1", "--command", "insert_content", "--content", "x", "--allow-deleting-content"}},
		{"bad command enum", []string{"page", "update", "p1", "--command", "replace_content_range", "--new-str", "x"}},
		{"properties-only forbids content", []string{"page", "update", "p1", "--properties", "{}", "--content", "x"}},
		{"nothing to do", []string{"page", "update", "p1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{}`, &got)
			defer srv.Close()
			code, _, stderr := run(t, srv, tc.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2 for an illegal combination", code)
			}
			if got.Path != "" {
				t.Errorf("no request must be sent for an illegal combination, got %s", got.Path)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Error("stderr is empty, want a usage error message")
			}
		})
	}
}

func TestPageUpdate_PropertiesOnly(t *testing.T) {
	var got capturedRequest
	// --json avoids the follow-up GET markdown; the single PATCH is the props one.
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "update", "p1", "--properties", `{"Status":{"status":{"name":"Done"}}}`, "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/pages/p1" {
		t.Errorf("request = %s %s, want PATCH /pages/p1 (properties-only, no markdown endpoint)", got.Method, got.Path)
	}
	// Properties PATCH runs on the default version, not the markdown version.
	assertAuth(t, got, notionVersion)
	body := bodyMap(t, got.Body)
	if _, ok := body["properties"]; !ok {
		t.Errorf("body = %v, want a properties field", body)
	}
}

func TestPageUpdate_IconSugar(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "update", "p1", "--icon", "🚀", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	icon, ok := body["icon"].(map[string]any)
	if !ok || icon["type"] != "emoji" || icon["emoji"] != "🚀" {
		t.Errorf("body.icon = %v, want an emoji wire object", body["icon"])
	}
}

func TestPageUpdate_CoverSugar(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "update", "p1", "--cover", "https://x/y.png", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	cover, ok := body["cover"].(map[string]any)
	if !ok || cover["type"] != "external" {
		t.Errorf("body.cover = %v, want an external wire object", body["cover"])
	}
}

// TestPageUpdate_PartialSuccess: content PATCH succeeds but the properties PATCH
// fails — the post-content markdown still lands on stdout, an explicit error
// goes to stderr, and the exit code is 1 (never a faked success).
func TestPageUpdate_PartialSuccess(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /pages/p1/markdown": {http.StatusOK, `{"markdown":"# done"}`},
		"PATCH /pages/p1":          {http.StatusBadRequest, `{"object":"error","code":"validation_error","message":"bad prop"}`},
	})
	defer srv.Close()

	result, stdout, stderr := runResult(t, srv,
		"page", "update", "p1", "--command", "replace_content", "--new-str", "# done", "--properties", `{"Nope":{}}`)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 for a partial success", result.ExitCode)
	}
	if stdout != "# done\n" {
		t.Errorf("stdout = %q, want the post-content markdown for the caller to reconcile", stdout)
	}
	if !strings.Contains(stderr, "content was written but properties were not") {
		t.Errorf("stderr = %q, want an explicit partial-success error", stderr)
	}
	if findReq(reqs, http.MethodPatch, "/pages/p1/markdown") == nil {
		t.Error("content PATCH was not sent")
	}
}

func TestPageReplace_Alias(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "replace", "p1", "--new-str", "hi")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/pages/p1/markdown" {
		t.Errorf("path = %q, want the markdown endpoint", got.Path)
	}
	body := bodyMap(t, got.Body)
	op, ok := body["replace_content"].(map[string]any)
	if !ok || body["type"] != "replace_content" || op["new_str"] != "hi" {
		t.Errorf("body = %v, want replace_content pinned by the alias with nested new_str", body)
	}
}

func TestPageEdit_Alias_ZipsPairs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "edit", "p1", "--old", "a", "--new", "1", "--old", "b", "--new", "2")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if body["type"] != "update_content" {
		t.Errorf("body.type = %v, want update_content", body["type"])
	}
	op, ok := body["update_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.update_content = %v, want a nested op object", body["update_content"])
	}
	ups, ok := op["content_updates"].([]any)
	if !ok || len(ups) != 2 {
		t.Fatalf("content_updates = %v, want two zipped pairs", op["content_updates"])
	}
	first := ups[0].(map[string]any)
	second := ups[1].(map[string]any)
	if first["old_str"] != "a" || first["new_str"] != "1" || second["old_str"] != "b" || second["new_str"] != "2" {
		t.Errorf("pairs = %v, want [{a,1},{b,2}] in order", ups)
	}
}

func TestPageEdit_Alias_CountMismatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "edit", "p1", "--old", "a", "--new", "1", "--old", "b")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a mismatched pair count", code)
	}
	if !strings.Contains(stderr, "counts must match") {
		t.Errorf("stderr = %q, want the pair-count error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent on a bad edit, got %s", got.Path)
	}
}

func TestPageInsert_Alias_At(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "insert", "p1", "--at", "start", "--content", "top")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	op, ok := body["insert_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.insert_content = %v, want a nested op object", body["insert_content"])
	}
	if body["type"] != "insert_content" || op["content"] != "top" {
		t.Errorf("body = %v, want insert_content/content", body)
	}
	pos, ok := op["position"].(map[string]any)
	if !ok || pos["type"] != "start" {
		t.Errorf("insert_content.position = %v, want {type:start}", op["position"])
	}
}

func TestPageAppend_Alias_End(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "append", "p1", "--content", "tail")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	op, ok := body["insert_content"].(map[string]any)
	if !ok {
		t.Fatalf("body.insert_content = %v, want a nested op object", body["insert_content"])
	}
	pos, ok := op["position"].(map[string]any)
	if body["type"] != "insert_content" || !ok || pos["type"] != "end" {
		t.Errorf("body = %v, want insert_content at end", body)
	}
}

// TestPageUpdate_ReplaceContent_File: --file supplies the replacement markdown
// for replace_content; the file contents land in the nested new_str.
func TestPageUpdate_ReplaceContent_File(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"markdown":"x"}`, &got)
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("# from file\n\nbody"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	code, _, stderr := run(t, srv, "page", "update", "p1", "--command", "replace_content", "--file", path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr)
	}
	if got.Path != "/pages/p1/markdown" {
		t.Errorf("path = %q, want the markdown endpoint", got.Path)
	}
	op, ok := bodyMap(t, got.Body)["replace_content"].(map[string]any)
	if !ok || op["new_str"] != "# from file\n\nbody" {
		t.Errorf("replace_content.new_str = %v, want the file contents", op["new_str"])
	}
}

// TestPageUpdate_File_MutuallyExclusive: giving both --file and the inline
// content flag is a fail-fast usage error (exit 2) with no request sent.
func TestPageUpdate_File_MutuallyExclusive(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	code, _, stderr := run(t, srv, "page", "update", "p1", "--command", "replace_content", "--new-str", "y", "--file", path)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for --file + --new-str", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q, want the mutual-exclusion error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestPageUpdate_File_BadPath: a nonexistent --file is a fail-fast usage error
// (exit 2) with no request sent.
func TestPageUpdate_File_BadPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	code, _, stderr := run(t, srv, "page", "update", "p1", "--command", "replace_content", "--file", missing)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a bad --file path", code)
	}
	if !strings.Contains(stderr, "read --file") {
		t.Errorf("stderr = %q, want a file-read usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestPageUpdate_PropertiesOnly_ConfirmationReadFails: the properties PATCH
// succeeds but the follow-up confirmation read (GET markdown) fails. The
// mutation already landed, so this is exit 0 with a note on stderr — never an
// exit-1 that would make an agent retry a completed write.
func TestPageUpdate_PropertiesOnly_ConfirmationReadFails(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /pages/p1": {http.StatusOK, `{"object":"page","id":"p1"}`},
		// GET /pages/p1/markdown falls through to the mux default 404 → read fails.
	})
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "page", "update", "p1", "--properties", `{"Status":{"status":{"name":"Done"}}}`)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (the mutation succeeded)", result.ExitCode)
	}
	if findReq(reqs, http.MethodPatch, "/pages/p1") == nil {
		t.Error("properties PATCH was not sent")
	}
	if !strings.Contains(stderr, "the update succeeded but reading back") {
		t.Errorf("stderr = %q, want a distinct read-back-failed note", stderr)
	}
}

// TestPageInsert_Alias_RejectsAllowDeleting: the insert alias pins
// insert_content, which forbids --allow-deleting-content; it fails fast (exit 2)
// rather than silently dropping the flag.
func TestPageInsert_Alias_RejectsAllowDeleting(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "insert", "p1", "--content", "x", "--allow-deleting-content")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for insert + --allow-deleting-content", code)
	}
	if !strings.Contains(stderr, "allow-deleting-content") {
		t.Errorf("stderr = %q, want the forbidden-flag error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestPageAppend_Alias_RejectsAllowDeleting: same as insert — append pins
// insert_content and forbids --allow-deleting-content.
func TestPageAppend_Alias_RejectsAllowDeleting(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "append", "p1", "--content", "x", "--allow-deleting-content")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for append + --allow-deleting-content", code)
	}
	if !strings.Contains(stderr, "allow-deleting-content") {
		t.Errorf("stderr = %q, want the forbidden-flag error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestPageEdit_Alias_RejectsFile: the edit alias pins update_content, which has
// no single-content input, so --file fails fast (exit 2).
func TestPageEdit_Alias_RejectsFile(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	code, _, stderr := run(t, srv, "page", "edit", "p1", "--old", "a", "--new", "b", "--file", path)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for edit + --file", code)
	}
	if !strings.Contains(stderr, "--file is not allowed") {
		t.Errorf("stderr = %q, want the forbidden-flag error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestPageMove_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /pages/p1/markdown": {http.StatusOK, `{"markdown":"x"}`}, // probe: types p1 as a page
		"POST /pages/p1/move":    {http.StatusOK, `{"object":"page","id":"p1"}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "move",
		"--page-or-database-ids", `["p1"]`,
		"--new-parent", `{"type":"page_id","page_id":"par1"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	move := findReq(reqs, http.MethodPost, "/pages/p1/move")
	if move == nil {
		t.Fatalf("POST /pages/p1/move was not sent; reqs=%v", reqs)
	}
	assertAuth(t, *move, markdownVersion)
	body := bodyMap(t, move.Body)
	parent, ok := body["parent"].(map[string]any)
	if !ok || parent["type"] != "page_id" || parent["page_id"] != "par1" {
		t.Errorf("move body.parent = %v, want the passed-through parent JSON", body["parent"])
	}
	if strings.TrimSpace(stdout) != "p1" {
		t.Errorf("stdout = %q, want the moved page id", stdout)
	}
}

// TestPageMove_RoutesDatabaseToUpdate: a database id is not a page-move — it is
// relocated via PATCH /v1/databases/{id} with the new parent (databases have no
// /move endpoint).
func TestPageMove_RoutesDatabaseToUpdate(t *testing.T) {
	var reqs []capturedRequest
	// Probe: page markdown 404, data source 404, database 200 → typed database.
	srv := newMux(t, &reqs, map[string]stub{
		"GET /databases/db1":   {http.StatusOK, `{"object":"database","id":"db1"}`},
		"PATCH /databases/db1": {http.StatusOK, `{"object":"database","id":"db1"}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "move",
		"--page-or-database-ids", `["db1"]`,
		"--new-parent", `{"type":"page_id","page_id":"par1"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a database move", code)
	}
	patch := findReq(reqs, http.MethodPatch, "/databases/db1")
	if patch == nil {
		t.Fatalf("PATCH /databases/db1 was not sent; reqs=%v", reqs)
	}
	if parent, ok := bodyMap(t, patch.Body)["parent"].(map[string]any); !ok || parent["page_id"] != "par1" {
		t.Errorf("PATCH body.parent = %v, want {page_id:par1}", bodyMap(t, patch.Body)["parent"])
	}
	if countReq(reqs, http.MethodPost, "/pages/db1/move") != 0 {
		t.Error("a page-move must not be sent for a database id")
	}
}

// TestPageMove_MultipleFanOut: two page ids each become one POST .../move, and
// both moved ids are listed on stdout.
func TestPageMove_MultipleFanOut(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /pages/p1/markdown": {http.StatusOK, `{"markdown":"x"}`}, // p1 types as a page
		"GET /pages/p2/markdown": {http.StatusOK, `{"markdown":"y"}`}, // p2 types as a page
		"POST /pages/p1/move":    {http.StatusOK, `{"object":"page","id":"p1"}`},
		"POST /pages/p2/move":    {http.StatusOK, `{"object":"page","id":"p2"}`},
	})
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "page", "move",
		"--page-or-database-ids", `["p1","p2"]`,
		"--new-parent", `{"type":"page_id","page_id":"par1"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr)
	}
	if countReq(reqs, http.MethodPost, "/pages/p1/move") != 1 || countReq(reqs, http.MethodPost, "/pages/p2/move") != 1 {
		t.Fatalf("want exactly one move per id; got p1=%d p2=%d",
			countReq(reqs, http.MethodPost, "/pages/p1/move"), countReq(reqs, http.MethodPost, "/pages/p2/move"))
	}
	lines := strings.Fields(stdout)
	if len(lines) != 2 || lines[0] != "p1" || lines[1] != "p2" {
		t.Errorf("stdout ids = %v, want [p1 p2]", lines)
	}
}

// TestPageMove_PartialFailure: the second move fails after the first landed —
// fail-fast with an explicit moved/unmoved split on stderr and exit 1, never a
// silent partial success.
func TestPageMove_PartialFailure(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /pages/p1/markdown": {http.StatusOK, `{"markdown":"x"}`},
		"GET /pages/p2/markdown": {http.StatusOK, `{"markdown":"y"}`},
		"POST /pages/p1/move":    {http.StatusOK, `{"object":"page","id":"p1"}`},
		"POST /pages/p2/move":    {http.StatusBadRequest, `{"object":"error","code":"validation_error","message":"nope"}`},
	})
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "page", "move",
		"--page-or-database-ids", `["p1","p2"]`,
		"--new-parent", `{"type":"page_id","page_id":"par1"}`)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 for a mid-loop move failure", result.ExitCode)
	}
	if countReq(reqs, http.MethodPost, "/pages/p1/move") != 1 || countReq(reqs, http.MethodPost, "/pages/p2/move") != 1 {
		t.Errorf("want both moves attempted; got p1=%d p2=%d",
			countReq(reqs, http.MethodPost, "/pages/p1/move"), countReq(reqs, http.MethodPost, "/pages/p2/move"))
	}
	if !strings.Contains(stderr, "moved 1/2 before failure") || !strings.Contains(stderr, "moved: [p1]") {
		t.Errorf("stderr = %q, want the moved/unmoved split", stderr)
	}
	if !strings.Contains(stderr, "failed moving p2") {
		t.Errorf("stderr = %q, want the failing id named", stderr)
	}
}

func TestPageDuplicate_WithNewParent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"dup1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "duplicate", "p1", "--new-parent", `{"type":"page_id","page_id":"par"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/pages" {
		t.Errorf("request = %s %s, want POST /pages", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	body := bodyMap(t, got.Body)
	tmpl, ok := body["template"].(map[string]any)
	// template.type must be "template_id" or the endpoint defaults to "none"
	// and copies nothing (silent no-op duplication).
	if !ok || tmpl["template_id"] != "p1" || tmpl["type"] != "template_id" {
		t.Errorf("body.template = %v, want {type:template_id, template_id:p1}", body["template"])
	}
	if _, ok := body["parent"].(map[string]any); !ok {
		t.Errorf("body.parent = %v, want the resolved parent object", body["parent"])
	}
	if strings.TrimSpace(stdout) != "dup1" {
		t.Errorf("stdout = %q, want the new page id", stdout)
	}
}

// TestPageDuplicate_Title: --title must land as a title property under
// `properties`, not as a top-level `title` string (which POST /v1/pages ignores).
func TestPageDuplicate_Title(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"dup1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "duplicate", "p1",
		"--new-parent", `{"type":"page_id","page_id":"par"}`, "--title", "Copy of X")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if _, ok := body["title"]; ok {
		t.Errorf("body must not carry a top-level title string, got %v", body["title"])
	}
	props, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("body.properties = %v, want a title property object", body["properties"])
	}
	titleProp, ok := props["title"].(map[string]any)
	if !ok {
		t.Fatalf("properties.title = %v, want a title property value", props["title"])
	}
	arr, ok := titleProp["title"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("properties.title.title = %v, want a one-element rich-text array", titleProp["title"])
	}
	text, _ := arr[0].(map[string]any)["text"].(map[string]any)
	if text["content"] != "Copy of X" {
		t.Errorf("title content = %v, want \"Copy of X\"", text["content"])
	}
}

// TestPageDuplicate_IgnoresAllowAsync: a template-based create is synchronous
// and Notion rejects allow_async without a markdown body, so `page duplicate`
// must never forward the global --allow-async into the request body.
func TestPageDuplicate_IgnoresAllowAsync(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"dup1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "duplicate", "p1",
		"--new-parent", `{"type":"page_id","page_id":"par"}`, "--allow-async")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, ok := bodyMap(t, got.Body)["allow_async"]; ok {
		t.Error("page duplicate must not send allow_async (template create has no markdown)")
	}
}

// TestPageCreate_AllowAsyncWithoutMarkdown: an element with no markdown body must
// not carry allow_async even under --allow-async (Notion rejects it).
func TestPageCreate_AllowAsyncWithoutMarkdown(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"np1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"},"properties":{}}]`, "--allow-async")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, ok := bodyMap(t, got.Body)["allow_async"]; ok {
		t.Error("create without markdown must not send allow_async")
	}
}

func TestPageDuplicate_CopiesSourceParent(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /pages/p1": {http.StatusOK, `{"object":"page","id":"p1","parent":{"type":"page_id","page_id":"orig"}}`},
		"POST /pages":   {http.StatusOK, `{"object":"page","id":"dup1"}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "page", "duplicate", "p1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/pages/p1") == nil {
		t.Error("source page GET was not sent; parent must be copied from the source")
	}
	create := findReq(reqs, http.MethodPost, "/pages")
	if create == nil {
		t.Fatalf("POST /pages was not sent")
	}
	body := bodyMap(t, create.Body)
	parent, ok := body["parent"].(map[string]any)
	if !ok || parent["page_id"] != "orig" {
		t.Errorf("body.parent = %v, want the source page's parent copied in", body["parent"])
	}
}

// TestPageMove_NewParentDataSourceURL: a database/data-source view URL (carries
// ?v=) must not be blindly typed data_source_id with the extracted id — the id
// is probed, and a data source types correctly as data_source_id.
func TestPageMove_NewParentDataSourceURL(t *testing.T) {
	dsID := "11111111-1111-1111-1111-111111111111"
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /pages/p1/markdown":    {http.StatusOK, `{"markdown":"x"}`},                             // p1 types as a page
		"GET /data_sources/" + dsID: {http.StatusOK, `{"object":"data_source","id":"` + dsID + `"}`}, // parent probe → data source
		"POST /pages/p1/move":       {http.StatusOK, `{"object":"page","id":"p1"}`},
	})
	defer srv.Close()

	parentURL := "https://www.notion.so/ws/11111111111111111111111111111111?v=22222222222222222222222222222222"
	code, _, stderr := run(t, srv, "page", "move",
		"--page-or-database-ids", `["p1"]`,
		"--new-parent", parentURL)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr)
	}
	move := findReq(reqs, http.MethodPost, "/pages/p1/move")
	if move == nil {
		t.Fatalf("move not sent; reqs=%v", reqs)
	}
	parent := bodyMap(t, move.Body)["parent"].(map[string]any)
	if parent["type"] != "data_source_id" || parent["data_source_id"] != dsID {
		t.Errorf("parent = %v, want a data_source_id wire resolved from the DB-view URL", parent)
	}
}

// TestPageMove_NewParentDatabaseURL_Rejected: a view URL whose id resolves to a
// database container is rejected (records live in its data sources), never
// wired as a data_source_id carrying the container id.
func TestPageMove_NewParentDatabaseURL_Rejected(t *testing.T) {
	dbID := "33333333-3333-3333-3333-333333333333"
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /databases/" + dbID: {http.StatusOK, `{"object":"database","id":"` + dbID + `"}`},
	})
	defer srv.Close()

	parentURL := "https://www.notion.so/ws/33333333333333333333333333333333?v=44444444444444444444444444444444"
	code, _, stderr := run(t, srv, "page", "move",
		"--page-or-database-ids", `["p1"]`,
		"--new-parent", parentURL)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a database-container parent", code)
	}
	if !strings.Contains(stderr, "database container") {
		t.Errorf("stderr = %q, want the database-container rejection", stderr)
	}
}

// TestPageMove_UnresolvableID_Usage: an id no probe can resolve yields
// move-specific guidance — never "pass --type" (move exposes no --type).
func TestPageMove_UnresolvableID_Usage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{}) // every endpoint 404s
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "move",
		"--page-or-database-ids", `["mystery"]`,
		"--new-parent", `{"type":"page_id","page_id":"par"}`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an unresolvable id", code)
	}
	if strings.Contains(stderr, "--type") {
		t.Errorf("stderr = %q, must not tell the caller to pass --type", stderr)
	}
	if !strings.Contains(stderr, "could not resolve") {
		t.Errorf("stderr = %q, want move-specific guidance", stderr)
	}
}

// TestFetch_Probe_CredentialRejected: a 401 on a probe must surface as an API
// error (exit 1) with CredentialRejected set — not be masked as a "pass --type"
// usage error — so OAuth token refresh (design 227) can trigger.
func TestFetch_Probe_CredentialRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"object":"error","code":"unauthorized","message":"token invalid"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "fetch", "p9")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (API error), not a masked usage error", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true — the probe must not swallow a 401")
	}
	if !strings.Contains(stderr, "unauthorized") {
		t.Errorf("stderr = %q, want the Notion auth error", stderr)
	}
	// The hard 401 aborts the chain at the first probe (page markdown).
	if got.Path != "/pages/p9/markdown" {
		t.Errorf("last request path = %q, want the chain to stop at the page probe", got.Path)
	}
}

// TestFetch_Probe_Indeterminate_Usage: a clean 404 on every probe is an
// indeterminate type — a usage error (exit 2) pointing at --type.
func TestFetch_Probe_Indeterminate_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"object":"error","code":"object_not_found","message":"nope"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "fetch", "p9")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when every probe 404s", code)
	}
	if !strings.Contains(stderr, "pass --type") {
		t.Errorf("stderr = %q, want the pass --type hint", stderr)
	}
}

// TestPageCreate_AsyncPoll: with --allow-async the create returns an async_task
// handle, which the CLI polls to a succeeded terminal state via
// GET /async_tasks/{id}, then emits the task's result.
func TestPageCreate_AsyncPoll(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /pages":          {http.StatusOK, `{"object":"async_task","id":"tk1","status":"queued","status_url":"https://api.notion.com/v1/async_tasks/tk1","poll_after_seconds":0}`},
		"GET /async_tasks/tk1": {http.StatusOK, `{"object":"async_task","id":"tk1","status":"succeeded","result":{"pages":[{"id":"pg1"}]}}`},
	})
	defer srv.Close()

	// allow_async is only valid when a markdown body is present, so the element
	// must carry content for the flag to be forwarded (and the async path taken).
	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"},"content":"# Big\n\nbody"}]`, "--allow-async", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	create := findReq(reqs, http.MethodPost, "/pages")
	if create == nil {
		t.Fatalf("POST /pages was not sent")
	}
	if body := bodyMap(t, create.Body); body["allow_async"] != true {
		t.Errorf("create body.allow_async = %v, want true", body["allow_async"])
	}
	if findReq(reqs, http.MethodGet, "/async_tasks/tk1") == nil {
		t.Error("the async task was not polled via GET /async_tasks/tk1")
	}
	if !strings.Contains(stdout, "pg1") {
		t.Errorf("stdout = %q, want the polled task result", stdout)
	}
}

// TestPageCreate_AsyncNote: without --allow-async an async_task handle is
// surfaced as a task id on stderr (exit 0), and the task is not polled.
func TestPageCreate_AsyncNote(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /pages": {http.StatusOK, `{"object":"async_task","id":"tk1","status":"queued"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"}}]`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "task get tk1") {
		t.Errorf("stderr = %q, want a note pointing at task get", stderr)
	}
	if countReq(reqs, http.MethodGet, "/async_tasks/tk1") != 0 {
		t.Error("the task must not be polled without --allow-async")
	}
}
