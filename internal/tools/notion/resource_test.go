package notion

import (
	"net/http"
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
	assertAuth(t, got, markdownVersion)
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

func TestDBQuery_Happy(t *testing.T) {
	var reqs []capturedRequest
	// GET /databases/ds1 is 404 (default) → not a database → query proceeds.
	srv := newMux(t, &reqs, map[string]stub{
		"POST /data_sources/ds1/query": {http.StatusOK, `{"object":"list","results":[],"has_more":false}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "db", "query", "ds1", "--filter", `{"property":"Done","checkbox":{"equals":false}}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := findReq(reqs, http.MethodPost, "/data_sources/ds1/query")
	if q == nil {
		t.Fatalf("query was not sent; reqs=%v", reqs)
	}
	assertAuth(t, *q, markdownVersion)
	body := bodyMap(t, q.Body)
	if _, ok := body["filter"].(map[string]any); !ok {
		t.Errorf("body.filter = %v, want the passthrough filter", body["filter"])
	}
}

func TestDBQuery_RejectsDatabaseID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /databases/db1": {http.StatusOK, `{"object":"database","id":"db1"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "db", "query", "db1")
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

func TestDBQuery_Pagination_All(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /data_sources/ds1/query": {http.StatusOK, `{"object":"list","results":[{"id":"r1"}],"has_more":false,"next_cursor":null}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "db", "query", "ds1", "--all")
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
	assertAuth(t, *p, markdownVersion)
	body := bodyMap(t, p.Body)
	if body["name"] != "Renamed" {
		t.Errorf("body.name = %v, want Renamed", body["name"])
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

	code, _, _ := run(t, srv, "view", "create", "--database-id", "db1", "--type", "table")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/views" {
		t.Errorf("request = %s %s, want POST /views", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
	body := bodyMap(t, got.Body)
	if body["type"] != "table" || body["database_id"] != "db1" {
		t.Errorf("body = %v, want type=table database_id=db1", body)
	}
}

func TestViewCreate_ParentExclusivity(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"none", []string{"view", "create", "--type", "table"}},
		{"two", []string{"view", "create", "--database-id", "db1", "--view-id", "v9", "--type", "table"}},
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

	code, _, stderr := run(t, srv, "view", "create", "--database-id", "db1", "--type", "kanban")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a bad view type", code)
	}
	if !strings.Contains(stderr, "must be one of") {
		t.Errorf("stderr = %q, want the enum error", stderr)
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

func TestCommentCreate_PlainText(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"comment","id":"c1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "comment", "create", "--page-id", "p1", "--content", "looks *good*")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/comments" {
		t.Errorf("request = %s %s, want POST /comments", got.Method, got.Path)
	}
	// Comment endpoint uses the default Notion-Version (not the markdown one).
	assertAuth(t, got, notionVersion)
	body := bodyMap(t, got.Body)
	parent, ok := body["parent"].(map[string]any)
	if !ok || parent["page_id"] != "p1" {
		t.Errorf("body.parent = %v, want {page_id:p1}", body["parent"])
	}
	rt, ok := body["rich_text"].([]any)
	if !ok || len(rt) != 1 {
		t.Fatalf("body.rich_text = %v, want a single run", body["rich_text"])
	}
	txt := rt[0].(map[string]any)["text"].(map[string]any)
	// The content is dropped in verbatim — no markdown parsing.
	if txt["content"] != "looks *good*" {
		t.Errorf("rich_text content = %v, want the verbatim plain text", txt["content"])
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

func TestTeamList_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "team", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/teams" {
		t.Errorf("request = %s %s, want GET /teams", got.Method, got.Path)
	}
	assertAuth(t, got, markdownVersion)
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
