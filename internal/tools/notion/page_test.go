package notion

import (
	"net/http"
	"strings"
	"testing"
)

func TestPageCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"pages":[{"id":"np1"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"par"},"properties":{"title":"Hello"},"content":"# Hi"}]`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/pages" {
		t.Errorf("request = %s %s, want POST /pages", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	// --pages is passed through verbatim under a "pages" key (MCP shape).
	body := bodyMap(t, got.Body)
	arr, ok := body["pages"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("body.pages = %v, want a one-element array", body["pages"])
	}
	if strings.TrimSpace(stdout) != "np1" {
		t.Errorf("stdout = %q, want the created page id", stdout)
	}
}

func TestPageCreate_JSONReturnsFull(t *testing.T) {
	var got capturedRequest
	response := `{"pages":[{"id":"np1"}]}`
	srv := newServer(t, http.StatusOK, response, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"}}]`, "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout != response+"\n" {
		t.Errorf("stdout = %q, want the full envelope under --json", stdout)
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
	// CLI --command maps to the REST field `type`.
	if body["type"] != "replace_content" {
		t.Errorf("body.type = %v, want replace_content", body["type"])
	}
	if body["new_str"] != "# updated" {
		t.Errorf("body.new_str = %v, want the replacement markdown", body["new_str"])
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
	if body["type"] != "insert_content" || body["content"] != "more" {
		t.Errorf("body = %v, want insert_content/content", body)
	}
	// --position is always sent, defaulting to end (never omitted).
	pos, ok := body["position"].(map[string]any)
	if !ok || pos["type"] != "end" {
		t.Errorf("body.position = %v, want {type:end} sent explicitly", body["position"])
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
	ups, ok := body["content_updates"].([]any)
	if !ok || len(ups) != 1 {
		t.Fatalf("body.content_updates = %v, want a one-element array", body["content_updates"])
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
	if body["type"] != "replace_content" || body["new_str"] != "hi" {
		t.Errorf("body = %v, want replace_content pinned by the alias", body)
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
	ups, ok := body["content_updates"].([]any)
	if !ok || len(ups) != 2 {
		t.Fatalf("content_updates = %v, want two zipped pairs", body["content_updates"])
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
	if body["type"] != "insert_content" || body["content"] != "top" {
		t.Errorf("body = %v, want insert_content/content", body)
	}
	pos, ok := body["position"].(map[string]any)
	if !ok || pos["type"] != "start" {
		t.Errorf("body.position = %v, want {type:start}", body["position"])
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
	pos, ok := body["position"].(map[string]any)
	if body["type"] != "insert_content" || !ok || pos["type"] != "end" {
		t.Errorf("body = %v, want insert_content at end", body)
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

func TestPageMove_RejectsDatabaseID(t *testing.T) {
	var reqs []capturedRequest
	// Probe: page markdown 404, data source 404, database 200 → typed database.
	srv := newMux(t, &reqs, map[string]stub{
		"GET /databases/db1": {http.StatusOK, `{"object":"database","id":"db1"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "page", "move",
		"--page-or-database-ids", `["db1"]`,
		"--new-parent", `{"type":"page_id","page_id":"par1"}`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a database id in move", code)
	}
	if !strings.Contains(stderr, "move only supports page ids") {
		t.Errorf("stderr = %q, want the page-id-only rejection", stderr)
	}
	if countReq(reqs, http.MethodPost, "/pages/db1/move") != 0 {
		t.Error("a move must not be sent for a database id")
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
	if !ok || tmpl["template_id"] != "p1" {
		t.Errorf("body.template = %v, want {template_id:p1}", body["template"])
	}
	if _, ok := body["parent"].(map[string]any); !ok {
		t.Errorf("body.parent = %v, want the resolved parent object", body["parent"])
	}
	if strings.TrimSpace(stdout) != "dup1" {
		t.Errorf("stdout = %q, want the new page id", stdout)
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

	code, stdout, _ := run(t, srv, "page", "create", "--pages", `[{"parent":{"page_id":"p"}}]`, "--allow-async", "--json")
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
