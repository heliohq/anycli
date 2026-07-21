package typefully

import (
	"net/http"
	"strings"
	"testing"
)

func TestDraftList_Filters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[],"count":0}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "draft", "list", "--social-set", "ss1",
		"--status", "scheduled", "--tag", "t1", "--order-by", "created", "--limit", "5", "--offset", "10")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/drafts" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("status") != "scheduled" || q.Get("tag") != "t1" || q.Get("order_by") != "created" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("limit") != "5" || q.Get("offset") != "10" {
		t.Errorf("paging query = %q", got.Query)
	}
	if !strings.Contains(stdout, `"count":0`) {
		t.Errorf("stdout = %q, want list envelope passthrough", stdout)
	}
}

func TestDraftCreate_TypedFlags_BuildsPlatformsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":9,"status":"draft"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "create", "--social-set", "ss1",
		"--text", "first post", "--text", "second post", "--publish-at", "now")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/social-sets/ss1/drafts" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}
	body := decodeBody(t, got.Body)
	if body["publish_at"] != "now" {
		t.Errorf("publish_at = %v, want now", body["publish_at"])
	}
	platforms, ok := body["platforms"].(map[string]any)
	if !ok {
		t.Fatalf("platforms not an object: %v", body["platforms"])
	}
	x, ok := platforms["x"].(map[string]any)
	if !ok {
		t.Fatalf("platforms.x not an object: %v", platforms["x"])
	}
	if x["enabled"] != true {
		t.Errorf("platforms.x.enabled = %v, want true", x["enabled"])
	}
	posts, ok := x["posts"].([]any)
	if !ok || len(posts) != 2 {
		t.Fatalf("posts = %v, want 2 (thread)", x["posts"])
	}
	first := posts[0].(map[string]any)
	if first["text"] != "first post" {
		t.Errorf("first post text = %v", first["text"])
	}
}

func TestDraftCreate_MediaAttachesToFirstPost(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "create", "--social-set", "ss1",
		"--text", "hi", "--platform", "linkedin", "--media-id", "m1", "--media-id", "m2")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	body := decodeBody(t, got.Body)
	platforms := body["platforms"].(map[string]any)
	li, ok := platforms["linkedin"].(map[string]any)
	if !ok {
		t.Fatalf("platforms.linkedin missing: %v", platforms)
	}
	posts := li["posts"].([]any)
	first := posts[0].(map[string]any)
	media, ok := first["media_ids"].([]any)
	if !ok || len(media) != 2 {
		t.Fatalf("media_ids = %v, want [m1 m2]", first["media_ids"])
	}
}

func TestDraftCreate_RawData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "create", "--social-set", "ss1",
		"--data", `{"platforms":{"threads":{"enabled":true,"posts":[{"text":"raw"}]}}}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	body := decodeBody(t, got.Body)
	platforms := body["platforms"].(map[string]any)
	if _, ok := platforms["threads"]; !ok {
		t.Errorf("raw --data body not passed through: %v", body)
	}
}

func TestDraftCreate_DataAndTypedFlagsExclusive_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "draft", "create", "--social-set", "ss1",
		"--data", `{"platforms":{}}`, "--text", "x")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for --data + typed flags", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP request should be sent on a usage error; got %s %s", got.Method, got.Path)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q, want mutual-exclusivity message", stderr)
	}
}

func TestDraftCreate_NoInput_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "create", "--social-set", "ss1")
	if code != 2 {
		t.Errorf("exit = %d, want 2 when neither --data nor --text given", code)
	}
}

func TestDraftCreate_InvalidJSON_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "create", "--social-set", "ss1", "--data", `{not json`)
	if code != 2 {
		t.Errorf("exit = %d, want 2 for invalid --data JSON", code)
	}
}

func TestDraftGet_ExcludeMarkers(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9,"publish_state":"finished","status":"published"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "get", "--social-set", "ss1", "--id", "9", "--exclude-comment-markers")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/drafts/9" {
		t.Errorf("path = %q", got.Path)
	}
	if parseQuery(t, got.Query).Get("exclude_comment_markers") != "true" {
		t.Errorf("query = %q, want exclude_comment_markers=true", got.Query)
	}
}

func TestDraftDelete_ReceiptOn204(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "draft", "delete", "--social-set", "ss1", "--id", "9")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/social-sets/ss1/drafts/9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"deleted":true`) {
		t.Errorf("stdout = %q, want delete receipt", stdout)
	}
}

func TestDraftUpdate_Patch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "draft", "update", "--social-set", "ss1", "--id", "9", "--data", `{"publish_at":"next-free-slot"}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/social-sets/ss1/drafts/9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if decodeBody(t, got.Body)["publish_at"] != "next-free-slot" {
		t.Errorf("patch body = %s", got.Body)
	}
}
