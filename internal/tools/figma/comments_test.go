package figma

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCommentsList(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"comments":[]}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server, "comments", "list", "--file-key", "abc", "--as-md")
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/files/abc/comments" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	query, _ := url.ParseQuery(got.Query)
	if query.Get("as_md") != "true" {
		t.Errorf("as_md = %q, want true", query.Get("as_md"))
	}
}

func TestCommentPost(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"id":"comment-1"}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server,
		"comments", "post", "--file-key", "abc", "--message", "Looks good",
		"--comment-id", "root-1", "--client-meta-json", `{"node_id":"1:2","node_offset":{"x":10,"y":20}}`,
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/files/abc/comments" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q", got.ContentType)
	}
	var payload commentPayload
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Message != "Looks good" || payload.CommentID != "root-1" {
		t.Errorf("payload = %+v", payload)
	}
	if payload.ClientMeta["node_id"] != "1:2" {
		t.Errorf("client_meta = %+v", payload.ClientMeta)
	}
}

func TestCommentPostRejectsInvalidClientMeta(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server,
		"comments", "post", "--file-key", "abc", "--message", "hello", "--client-meta-json", `[]`,
	)
	if code != 1 || !strings.Contains(stderr, "must be a JSON object") {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "" {
		t.Errorf("request unexpectedly sent to %s", got.Path)
	}
}

func TestCommentDeleteEmitsJSONForEmptySuccess(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, "", nil, &got)
	defer server.Close()

	code, stdout, stderr := runService(t, server,
		"comments", "delete", "--file-key", "abc", "--comment-id", "comment-1",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/files/abc/comments/comment-1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if stdout != "{}\n" {
		t.Errorf("stdout = %q, want empty JSON object", stdout)
	}
}
