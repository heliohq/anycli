package pinterest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPinCreateImageBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"p1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"pin", "create",
		"--board-id", "b1",
		"--image-url", "https://example.com/a.png",
		"--title", "Hi",
		"--description", "desc",
		"--link", "https://example.com",
		"--section-id", "s1",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/pins" {
		t.Errorf("request = %s %s, want POST /pins", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["board_id"] != "b1" || body["title"] != "Hi" || body["description"] != "desc" ||
		body["link"] != "https://example.com" || body["board_section_id"] != "s1" {
		t.Errorf("body = %v, want fields set", body)
	}
	ms, ok := body["media_source"].(map[string]any)
	if !ok || ms["source_type"] != "image_url" || ms["url"] != "https://example.com/a.png" {
		t.Errorf("media_source = %v, want image_url shape", body["media_source"])
	}
}

func TestPinCreateRequiresBoardAndImage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "pin", "create", "--image-url", "https://x/a.png")
	if exit != 2 || !strings.Contains(stderr, "--board-id is required") {
		t.Errorf("missing board: exit=%d stderr=%q", exit, stderr)
	}

	exit, _, stderr = run(t, srv, "pin", "create", "--board-id", "b1")
	if exit != 2 || !strings.Contains(stderr, "--image-url is required") {
		t.Errorf("missing image: exit=%d stderr=%q", exit, stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, saw %s %s", got.Method, got.Path)
	}
}

func TestPinCreateOmitsEmptyOptionalFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	run(t, srv, "pin", "create", "--board-id", "b1", "--image-url", "https://x/a.png")
	body := decodeBody(t, got.Body)
	for _, f := range []string{"title", "description", "link", "board_section_id"} {
		if _, ok := body[f]; ok {
			t.Errorf("empty %s should be omitted", f)
		}
	}
}

func TestPinGetListDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"p1"}`, &got)
	defer srv.Close()

	run(t, srv, "pin", "get", "p1")
	if got.Path != "/pins/p1" || got.Method != http.MethodGet {
		t.Errorf("get: %s %s", got.Method, got.Path)
	}

	run(t, srv, "pin", "list", "--bookmark", "bm")
	if got.Path != "/pins" {
		t.Errorf("list path = %q", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("bookmark") != "bm" {
		t.Errorf("list query = %q, want bookmark=bm", got.Query)
	}

	run(t, srv, "pin", "delete", "p1")
	if got.Path != "/pins/p1" || got.Method != http.MethodDelete {
		t.Errorf("delete: %s %s", got.Method, got.Path)
	}
}

func TestPinDeleteEmptyReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "pin", "delete", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stdout, `"deleted":true`) {
		t.Errorf("stdout = %q, want receipt", stdout)
	}
}
