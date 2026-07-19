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

func TestDBCreate_WrapsProperties(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"database","id":"db1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "db", "create",
		"--parent", `{"type":"page_id","page_id":"par"}`,
		"--title", "Tasks",
		"--properties", `{"Name":{"title":{}},"Done":{"checkbox":{}}}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/databases" {
		t.Errorf("request = %s %s, want POST /databases", got.Method, got.Path)
	}
	assertAuth(t, got, notionVersion)
	body := bodyMap(t, got.Body)
	// Schema is wrapped into initial_data_source.properties, not top-level.
	if _, ok := body["properties"]; ok {
		t.Errorf("body has a top-level properties field; the 2026-03-11 model wraps it")
	}
	ids, ok := body["initial_data_source"].(map[string]any)
	if !ok {
		t.Fatalf("body.initial_data_source = %v, want a wrapper object", body["initial_data_source"])
	}
	if _, ok := ids["properties"].(map[string]any); !ok {
		t.Errorf("initial_data_source.properties = %v, want the schema", ids["properties"])
	}
	// title must be a rich-text array (POST /v1/databases rejects a bare string).
	titleArr, ok := body["title"].([]any)
	if !ok || len(titleArr) != 1 {
		t.Fatalf("body.title = %v, want a one-element rich-text array", body["title"])
	}
	run0, _ := titleArr[0].(map[string]any)
	text, _ := run0["text"].(map[string]any)
	if text["content"] != "Tasks" {
		t.Errorf("body.title[0].text.content = %v, want Tasks", text["content"])
	}
}

func TestDataSourceQuery_Happy(t *testing.T) {
	var reqs []capturedRequest
	// GET /databases/ds1 is 404 (default) → not a database → query proceeds.
	srv := newMux(t, &reqs, map[string]stub{
		"POST /data_sources/ds1/query": {http.StatusOK, `{"object":"list","results":[],"has_more":false}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "data-source", "query", "ds1", "--filter", `{"property":"Done","checkbox":{"equals":false}}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := findReq(reqs, http.MethodPost, "/data_sources/ds1/query")
	if q == nil {
		t.Fatalf("query was not sent; reqs=%v", reqs)
	}
	assertAuth(t, *q, notionVersion)
	body := bodyMap(t, q.Body)
	if _, ok := body["filter"].(map[string]any); !ok {
		t.Errorf("body.filter = %v, want the passthrough filter", body["filter"])
	}
}

func TestDataSourceQuery_RejectsDatabaseID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /databases/db1": {http.StatusOK, `{"object":"database","id":"db1"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "data-source", "query", "db1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a database id", code)
	}
	if !strings.Contains(stderr, "only accepts a data-source id") {
		t.Errorf("stderr = %q, want the data-source-only guard", stderr)
	}
	if countReq(reqs, http.MethodPost, "/data_sources/db1/query") != 0 {
		t.Error("no query must be sent when the id is a database")
	}
}

func TestDataSourceQuery_Pagination_All(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /data_sources/ds1/query": {http.StatusOK, `{"object":"list","results":[{"id":"r1"}],"has_more":false,"next_cursor":null}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "data-source", "query", "ds1", "--all")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, `"r1"`) {
		t.Errorf("stdout = %q, want the accumulated result", stdout)
	}
}

func TestDataSourceUpdate_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /data_sources/ds1": {http.StatusOK, `{"object":"data_source","id":"ds1"}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "data-source", "update", "ds1", "--name", "Renamed",
		"--properties", `{"Priority":{"select":{"options":[{"name":"High"}]}}}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	p := findReq(reqs, http.MethodPatch, "/data_sources/ds1")
	if p == nil {
		t.Fatalf("PATCH /data_sources/ds1 was not sent; reqs=%v", reqs)
	}
	assertAuth(t, *p, notionVersion)
	body := bodyMap(t, p.Body)
	// --name is mapped to the rich-text `title` array; there is no `name` field
	// on PATCH /v1/data_sources.
	if _, ok := body["name"]; ok {
		t.Errorf("body.name must not be sent (no such field); got %v", body["name"])
	}
	title, ok := body["title"].([]any)
	if !ok || len(title) == 0 {
		t.Fatalf("body.title = %v, want a rich-text array carrying the rename", body["title"])
	}
	run0, _ := title[0].(map[string]any)
	txt, _ := run0["text"].(map[string]any)
	if txt["content"] != "Renamed" {
		t.Errorf("body.title[0].text.content = %v, want Renamed", txt["content"])
	}
	if _, ok := body["properties"].(map[string]any); !ok {
		t.Errorf("body.properties = %v, want the passthrough schema patch", body["properties"])
	}
}

func TestDataSourceUpdate_NoFields_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "data-source", "update", "ds1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when nothing is given", code)
	}
	if !strings.Contains(stderr, "at least one of") {
		t.Errorf("stderr = %q, want the missing-field usage error", stderr)
	}
}

func TestViewCreate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"view","id":"v1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "view", "create", "--database-id", "db1", "--data-source-id", "ds1", "--name", "Recent", "--type", "table")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/views" {
		t.Errorf("request = %s %s, want POST /views", got.Method, got.Path)
	}
	assertAuth(t, got, notionVersion)
	body := bodyMap(t, got.Body)
	// POST /v1/views requires database_id + data_source_id + name + type.
	if body["type"] != "table" || body["database_id"] != "db1" || body["data_source_id"] != "ds1" || body["name"] != "Recent" {
		t.Errorf("body = %v, want type=table database_id=db1 data_source_id=ds1 name=Recent", body)
	}
}

// TestViewCreate_RequiresDataSource: the --database-id path must supply
// --data-source-id (POST /v1/views rejects a missing data_source_id).
func TestViewCreate_RequiresDataSource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "view", "create", "--database-id", "db1", "--name", "X", "--type", "table")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --data-source-id", code)
	}
	if !strings.Contains(stderr, "data-source-id") {
		t.Errorf("stderr = %q, want the missing data-source-id error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestViewCreate_ParentExclusivity(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"none", []string{"view", "create", "--data-source-id", "ds1", "--name", "X", "--type", "table"}},
		{"two", []string{"view", "create", "--database-id", "db1", "--view-id", "v9", "--data-source-id", "ds1", "--name", "X", "--type", "table"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{}`, &got)
			defer srv.Close()
			code, _, stderr := run(t, srv, tc.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2 for a bad parent count", code)
			}
			if !strings.Contains(stderr, "exactly one of") {
				t.Errorf("stderr = %q, want the exclusivity error", stderr)
			}
			if got.Path != "" {
				t.Errorf("no request must be sent, got %s", got.Path)
			}
		})
	}
}

func TestViewCreate_BadType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "view", "create", "--database-id", "db1", "--data-source-id", "ds1", "--name", "X", "--type", "kanban")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a bad view type", code)
	}
	if !strings.Contains(stderr, "must be one of") {
		t.Errorf("stderr = %q, want the enum error", stderr)
	}
}

// TestViewCreate_CreateDatabaseRequiresDataSource: the --create-database parent
// path also requires --data-source-id (verified live: POST /v1/views 400s
// without it), so cobra must reject it before any request.
func TestViewCreate_CreateDatabaseRequiresDataSource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	code, _, stderr := run(t, srv, "view", "create", "--create-database", `{}`, "--name", "X", "--type", "table")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --data-source-id", code)
	}
	if !strings.Contains(stderr, "data-source-id") {
		t.Errorf("stderr = %q, want the required-flag error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// TestViewCreate_ConfigurationAndQuickFilters: --configuration / --quick-filters
// pass through verbatim into the create body.
func TestViewCreate_ConfigurationAndQuickFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"view","id":"v1"}`, &got)
	defer srv.Close()
	code, _, _ := run(t, srv, "view", "create", "--database-id", "db1", "--data-source-id", "ds1",
		"--name", "N", "--type", "board",
		"--configuration", `{"type":"table","group_by":{"type":"select"}}`,
		"--quick-filters", `{"and":[]}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if _, ok := body["configuration"].(map[string]any); !ok {
		t.Errorf("body.configuration = %v, want the passthrough object", body["configuration"])
	}
	if _, ok := body["quick_filters"].(map[string]any); !ok {
		t.Errorf("body.quick_filters = %v, want the passthrough object", body["quick_filters"])
	}
}

// TestViewUpdate_ConfigurationAndQuickFilters: --configuration / --quick-filters
// pass through verbatim into the update body.
func TestViewUpdate_ConfigurationAndQuickFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"view","id":"v1"}`, &got)
	defer srv.Close()
	code, _, _ := run(t, srv, "view", "update", "v1",
		"--configuration", `{"type":"table"}`, "--quick-filters", `{"and":[]}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if _, ok := body["configuration"]; !ok {
		t.Errorf("body.configuration missing; got %v", body)
	}
	if _, ok := body["quick_filters"]; !ok {
		t.Errorf("body.quick_filters missing; got %v", body)
	}
}

func TestViewUpdate_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"view","id":"v1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "view", "update", "v1", "--name", "Grid", "--filters", `{"and":[]}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/views/v1" {
		t.Errorf("request = %s %s, want PATCH /views/v1", got.Method, got.Path)
	}
	body := bodyMap(t, got.Body)
	if body["name"] != "Grid" {
		t.Errorf("body.name = %v, want Grid", body["name"])
	}
	if _, ok := body["filters"].(map[string]any); !ok {
		t.Errorf("body.filters = %v, want the passthrough filters", body["filters"])
	}
}

// TestCommentCreate_PageMarkdown: --content is sent via the `markdown` field
// (not a plain-text rich_text run), targeting a page.
func TestCommentCreate_PageMarkdown(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"comment","id":"c1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "comment", "create", "--page-id", "p1", "--content", "looks **good**")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/comments" {
		t.Errorf("request = %s %s, want POST /comments", got.Method, got.Path)
	}
	assertAuth(t, got, notionVersion)
	body := bodyMap(t, got.Body)
	if parent, ok := body["parent"].(map[string]any); !ok || parent["page_id"] != "p1" {
		t.Errorf("body.parent = %v, want {page_id:p1}", body["parent"])
	}
	if body["markdown"] != "looks **good**" {
		t.Errorf("body.markdown = %v, want the markdown content", body["markdown"])
	}
	if _, ok := body["rich_text"]; ok {
		t.Errorf("body must send markdown, not rich_text; got %v", body["rich_text"])
	}
}

// TestCommentCreate_Block: --block-id targets a specific block.
func TestCommentCreate_Block(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"comment","id":"c1"}`, &got)
	defer srv.Close()
	code, _, _ := run(t, srv, "comment", "create", "--block-id", "b1", "--content", "on this block")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if parent, ok := bodyMap(t, got.Body)["parent"].(map[string]any); !ok || parent["block_id"] != "b1" {
		t.Errorf("body.parent = %v, want {block_id:b1}", bodyMap(t, got.Body)["parent"])
	}
}

// TestCommentCreate_Discussion: --discussion-id replies into a thread (top-level
// discussion_id, no parent).
func TestCommentCreate_Discussion(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"comment","id":"c1"}`, &got)
	defer srv.Close()
	code, _, _ := run(t, srv, "comment", "create", "--discussion-id", "d1", "--content", "reply")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	if body["discussion_id"] != "d1" {
		t.Errorf("body.discussion_id = %v, want d1", body["discussion_id"])
	}
	if _, ok := body["parent"]; ok {
		t.Errorf("discussion reply must not send parent; got %v", body["parent"])
	}
}

// TestCommentCreate_TargetExclusivity: exactly one target is required.
func TestCommentCreate_TargetExclusivity(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	// none
	code, _, stderr := run(t, srv, "comment", "create", "--content", "x")
	if code != 2 || !strings.Contains(stderr, "exactly one of") {
		t.Errorf("no-target: code=%d stderr=%q, want 2 + exclusivity error", code, stderr)
	}
	// two
	code, _, stderr = run(t, srv, "comment", "create", "--page-id", "p1", "--block-id", "b1", "--content", "x")
	if code != 2 || !strings.Contains(stderr, "exactly one of") {
		t.Errorf("two-targets: code=%d stderr=%q, want 2 + exclusivity error", code, stderr)
	}
}

func TestCommentList_BlockID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[],"has_more":false}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "comment", "list", "p1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/comments" {
		t.Errorf("request = %s %s, want GET /comments", got.Method, got.Path)
	}
	if got.Query.Get("block_id") != "p1" {
		t.Errorf("block_id = %q, want p1", got.Query.Get("block_id"))
	}
}

func TestUserGet_Self(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"user","id":"me"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get", "self")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/users/me" {
		t.Errorf("request = %s %s, want GET /users/me", got.Method, got.Path)
	}
}

// TestUserGet_ByID resolves a specific user id to GET /v1/users/{id} (agents
// resolve created_by/last_edited_by ids, incl. guests that list omits).
func TestUserGet_ByID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"user","id":"u1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get", "u1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/users/u1" {
		t.Errorf("request = %s %s, want GET /users/u1", got.Method, got.Path)
	}
}

func TestUserGet_List(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[],"has_more":false}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/users" {
		t.Errorf("path = %q, want /users", got.Path)
	}
}

// TestUserGet_List_Paginates: the no-arg form must enumerate every user by
// following next_cursor (spec §user "列出所有用户(分页)"), not return only the
// first page.
func TestUserGet_List_Paginates(t *testing.T) {
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, capturedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: body})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start_cursor") == "cur2" {
			_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"u2"}],"has_more":false,"next_cursor":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"u1"}],"has_more":true,"next_cursor":"cur2"}`))
	}))
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "get"}, map[string]string{EnvToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if len(reqs) != 2 {
		t.Fatalf("made %d requests, want 2 (no-arg must follow next_cursor)", len(reqs))
	}
	var merged struct {
		Results []map[string]any `json:"results"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(out.Bytes(), &merged); err != nil {
		t.Fatalf("merged output not JSON: %v", err)
	}
	if len(merged.Results) != 2 || merged.HasMore {
		t.Errorf("merged = %+v, want both users aggregated and has_more=false", merged)
	}
}

func TestUserGet_Query_Filters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"object":"list","results":[{"name":"Alice","person":{"email":"a@x.com"}},{"name":"Bob","person":{"email":"b@x.com"}}],"has_more":false,"next_cursor":null}`,
		&got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "get", "--query", "ALI")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Alice") || strings.Contains(stdout, "Bob") {
		t.Errorf("stdout = %q, want only the case-insensitive substring match on Alice", stdout)
	}
}

func TestUserGet_SelfAndQuery_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "user", "get", "self", "--query", "x")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for self + --query", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q, want the exclusivity error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestTaskGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"async_task","id":"tk1","status":"succeeded"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "task", "get", "tk1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/async_tasks/tk1" {
		t.Errorf("request = %s %s, want GET /async_tasks/tk1", got.Method, got.Path)
	}
}

// TestFetch_Probe_DataSourceWins covers the probe order beyond the page case:
// the page-markdown probe 404s, so the data-source probe wins and the id types
// as a data source (JSON output).
func TestFetch_Probe_DataSourceWins(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /data_sources/x1": {http.StatusOK, `{"object":"data_source","id":"x1"}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "fetch", "x1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/pages/x1/markdown") == nil {
		t.Error("the page-markdown probe was not attempted first")
	}
	if !strings.Contains(stdout, `"data_source"`) {
		t.Errorf("stdout = %q, want the data-source JSON after the probe fell through", stdout)
	}
}

func TestDBQuery_Removed(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "db", "query", "ds1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for removed db query", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want unknown command", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}
