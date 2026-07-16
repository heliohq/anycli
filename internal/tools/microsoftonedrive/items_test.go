package microsoftonedrive

import (
	"net/http"
	"strings"
	"testing"
)

func TestItemsList_HumanAndJSON(t *testing.T) {
	body := `{"value":[` +
		`{"id":"f1","name":"Docs","folder":{"childCount":2},"lastModifiedDateTime":"2026-01-01T00:00:00Z"},` +
		`{"id":"i1","name":"report.pdf","size":1024,"file":{"mimeType":"application/pdf"},"lastModifiedDateTime":"2026-01-02T00:00:00Z"}` +
		`],"@odata.nextLink":"` + "NEXTLINK" + `"}`
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/root/children": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "items", "list")
	if !strings.Contains(stdout, "Docs/") || !strings.Contains(stdout, "report.pdf") {
		t.Errorf("human output = %q, want folder + file rows", stdout)
	}
	if !strings.Contains(stdout, "next page: NEXTLINK") {
		t.Errorf("human output = %q, want next page line", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/drive/root/children")
	if got.Auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
	if !strings.Contains(got.Query, "%24top=20") && !strings.Contains(got.Query, "$top=20") {
		t.Errorf("query = %q, want $top=20", got.Query)
	}

	stdout = f.runOK(t, "items", "list", "--json")
	if !strings.Contains(stdout, `"report.pdf"`) {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestItemsList_ByPath(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/root:/Documents/2026:/children": {http.StatusOK, `{"value":[]}`},
	})
	stdout := f.runOK(t, "items", "list", "--path", "Documents/2026")
	if !strings.Contains(stdout, "no items") {
		t.Errorf("output = %q, want empty listing", stdout)
	}
}

func TestItemsList_ByParentID(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/items/parent1/children": {http.StatusOK, `{"value":[]}`},
	})
	f.runOK(t, "items", "list", "--parent", "parent1")
	f.last(t, "GET", "/v1.0/me/drive/items/parent1/children")
}

func TestItemsList_Page(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/root/children": {http.StatusOK, `{"value":[]}`},
	})
	// Point --page at a concrete nextLink on the fixture server so the raw
	// URL is fetched verbatim rather than rebuilt with $top.
	page := f.srv.URL + "/v1.0/me/drive/root/children?%24skiptoken=abc"
	f.runOK(t, "items", "list", "--page", page)
	got := f.last(t, "GET", "/v1.0/me/drive/root/children")
	if !strings.Contains(got.Query, "skiptoken=abc") {
		t.Errorf("query = %q, want the skiptoken from the nextLink", got.Query)
	}
	if strings.Contains(got.Query, "top") {
		t.Errorf("query = %q, --page must not re-append $top", got.Query)
	}
}

func TestItemsGet_Human(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/items/id1": {http.StatusOK, `{"id":"id1","name":"a.txt","size":5,"file":{"mimeType":"text/plain"},"webUrl":"https://x","lastModifiedDateTime":"2026-01-01T00:00:00Z"}`},
	})
	stdout := f.runOK(t, "items", "get", "id1")
	if !strings.Contains(stdout, "a.txt") || !strings.Contains(stdout, "text/plain") || !strings.Contains(stdout, "5 bytes") {
		t.Errorf("output = %q, want name/mime/size", stdout)
	}
}

func TestItemsGet_ByPath(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/root:/a/b.txt:": {http.StatusOK, `{"id":"id2","name":"b.txt","size":1}`},
	})
	f.runOK(t, "items", "get", "--path", "a/b.txt")
	f.last(t, "GET", "/v1.0/me/drive/root:/a/b.txt:")
}

func TestItemsMkdir(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/drive/root/children": {http.StatusCreated, `{"id":"newf","name":"Reports"}`},
	})
	stdout := f.runOK(t, "items", "mkdir", "--name", "Reports")
	if !strings.Contains(stdout, "created folder Reports") {
		t.Errorf("output = %q, want created folder", stdout)
	}
	got := f.last(t, "POST", "/v1.0/me/drive/root/children")
	if !strings.Contains(string(got.Body), `"folder"`) || !strings.Contains(string(got.Body), `"Reports"`) {
		t.Errorf("body = %q, want folder facet + name", got.Body)
	}
}

func TestItemsMove(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /v1.0/me/drive/items/id1": {http.StatusOK, `{"id":"id1","name":"renamed.txt"}`},
	})
	f.runOK(t, "items", "move", "id1", "--to", "dir9", "--name", "renamed.txt")
	got := f.last(t, "PATCH", "/v1.0/me/drive/items/id1")
	if !strings.Contains(string(got.Body), `"parentReference"`) || !strings.Contains(string(got.Body), `"dir9"`) {
		t.Errorf("body = %q, want parentReference id", got.Body)
	}
	if !strings.Contains(string(got.Body), `"renamed.txt"`) {
		t.Errorf("body = %q, want new name", got.Body)
	}
}

func TestItemsRename(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /v1.0/me/drive/items/id1": {http.StatusOK, `{"id":"id1","name":"new.txt"}`},
	})
	f.runOK(t, "items", "rename", "id1", "--name", "new.txt")
	got := f.last(t, "PATCH", "/v1.0/me/drive/items/id1")
	if !strings.Contains(string(got.Body), `"new.txt"`) || strings.Contains(string(got.Body), "parentReference") {
		t.Errorf("body = %q, want name-only patch", got.Body)
	}
}

func TestItemsShare_DefaultScopeOrganization(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/drive/items/id1/createLink": {http.StatusCreated, `{"link":{"type":"view","scope":"organization","webUrl":"https://share"}}`},
	})
	stdout := f.runOK(t, "items", "share", "id1")
	if !strings.Contains(stdout, "https://share") || !strings.Contains(stdout, "organization") {
		t.Errorf("output = %q, want link + org scope", stdout)
	}
	got := f.last(t, "POST", "/v1.0/me/drive/items/id1/createLink")
	if !strings.Contains(string(got.Body), `"scope":"organization"`) {
		t.Errorf("body = %q, want default organization scope (not anonymous)", got.Body)
	}
	if !strings.Contains(string(got.Body), `"type":"view"`) {
		t.Errorf("body = %q, want default view type", got.Body)
	}
}

func TestItemsShare_Anonymous(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/drive/items/id1/createLink": {http.StatusCreated, `{"link":{"type":"edit","scope":"anonymous","webUrl":"https://pub"}}`},
	})
	f.runOK(t, "items", "share", "id1", "--type", "edit", "--scope", "anonymous")
	got := f.last(t, "POST", "/v1.0/me/drive/items/id1/createLink")
	if !strings.Contains(string(got.Body), `"scope":"anonymous"`) {
		t.Errorf("body = %q, want anonymous scope when requested", got.Body)
	}
}

func TestItemsDelete_Multiple(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /v1.0/me/drive/items/id1": {http.StatusNoContent, ``},
		"DELETE /v1.0/me/drive/items/id2": {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "items", "delete", "id1", "id2")
	if !strings.Contains(stdout, "deleted 2 item(s)") {
		t.Errorf("output = %q, want 2 deleted", stdout)
	}
	f.last(t, "DELETE", "/v1.0/me/drive/items/id1")
	f.last(t, "DELETE", "/v1.0/me/drive/items/id2")
}
