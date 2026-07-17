package bitly

import (
	"net/http"
	"strings"
	"testing"
)

func TestLinkShorten_AutoResolvesGroup(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/user":    {status: http.StatusOK, response: `{"default_group_guid":"Bg-default"}`},
		"/shorten": {status: http.StatusOK, response: `{"link":"https://bit.ly/x"}`},
	}, captured)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "link", "shorten", "--long-url", "https://example.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, ok := captured["/user"]; !ok {
		t.Fatal("expected GET /user for group auto-resolution")
	}
	req := captured["/shorten"]
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", req.Auth)
	}
	body := decodeBody(t, req.Body)
	if body["long_url"] != "https://example.com" {
		t.Errorf("long_url = %v", body["long_url"])
	}
	if body["group_guid"] != "Bg-default" {
		t.Errorf("group_guid = %v, want auto-resolved Bg-default", body["group_guid"])
	}
	if body["domain"] != "bit.ly" {
		t.Errorf("domain = %v, want bit.ly", body["domain"])
	}
	if body["force_new_link"] != false {
		t.Errorf("force_new_link = %v, want false", body["force_new_link"])
	}
	if !strings.Contains(stdout, `"link"`) {
		t.Errorf("stdout = %q, want passthrough", stdout)
	}
}

func TestLinkShorten_ExplicitGroupSkipsUserCall(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/shorten": {status: http.StatusOK, response: `{"link":"x"}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "shorten", "--long-url", "https://e.com", "--group", "Bg-explicit")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, ok := captured["/user"]; ok {
		t.Error("did not expect GET /user when --group is explicit")
	}
	if body := decodeBody(t, captured["/shorten"].Body); body["group_guid"] != "Bg-explicit" {
		t.Errorf("group_guid = %v, want Bg-explicit", body["group_guid"])
	}
}

func TestLinkCreate_FullBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"bit.ly/x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "create",
		"--long-url", "https://e.com", "--group", "Bg1", "--title", "T",
		"--tags", "a", "--tags", "b", "--keyword", "kw",
		"--deeplinks-json", `[{"app_uri_path":"/x"}]`, "--expiration-at", "2030-01-01T00:00:00+0000",
		"--force-new-link")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/bitlinks" {
		t.Errorf("request = %s %s, want POST /bitlinks", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["title"] != "T" || body["keyword"] != "kw" {
		t.Errorf("body = %v", body)
	}
	if body["force_new_link"] != true {
		t.Errorf("force_new_link = %v, want true", body["force_new_link"])
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [a b]", body["tags"])
	}
	if _, ok := body["deeplinks"].([]any); !ok {
		t.Errorf("deeplinks = %v, want array passthrough", body["deeplinks"])
	}
}

func TestLinkCreate_InvalidDeeplinksJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "link", "create", "--long-url", "https://e.com",
		"--group", "Bg1", "--deeplinks-json", `{not json`)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want JSON validation error", stderr)
	}
}

func TestLinkExpand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"long_url":"https://e.com"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "expand", "--bitlink", "bit.ly/2ab")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/expand" {
		t.Errorf("request = %s %s, want POST /expand", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["bitlink_id"] != "bit.ly/2ab" {
		t.Errorf("bitlink_id = %v", body["bitlink_id"])
	}
}

func TestLinkGet_LiteralSlashPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"bit.ly/2ab"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "get", "--bitlink", "bit.ly/2ab")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/bitlinks/bit.ly/2ab" {
		t.Errorf("path = %q, want /bitlinks/bit.ly/2ab (literal slash, not encoded)", got.Path)
	}
}

func TestLinkUpdate_OnlyChangedFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"bit.ly/2ab"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "update", "--bitlink", "bit.ly/2ab", "--title", "New", "--archived")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/bitlinks/bit.ly/2ab" {
		t.Errorf("request = %s %s, want PATCH /bitlinks/bit.ly/2ab", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["title"] != "New" {
		t.Errorf("title = %v", body["title"])
	}
	if body["archived"] != true {
		t.Errorf("archived = %v, want true", body["archived"])
	}
	if _, ok := body["long_url"]; ok {
		t.Errorf("long_url should be omitted when unset, body = %v", body)
	}
}

func TestLinkList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"links":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "list", "--group", "Bg1", "--size", "10",
		"--query", "promo", "--archived", "both", "--tags", "t1", "--tags", "t2")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/groups/Bg1/bitlinks" {
		t.Errorf("path = %q, want /groups/Bg1/bitlinks", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("size") != "10" || q.Get("query") != "promo" || q.Get("archived") != "both" {
		t.Errorf("query = %q", got.Query)
	}
	if tags := q["tags"]; len(tags) != 2 {
		t.Errorf("tags = %v, want two values", tags)
	}
}
