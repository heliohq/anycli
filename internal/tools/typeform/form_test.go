package typeform

import (
	"net/http"
	"strings"
	"testing"
)

func TestFormListPassesQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms": {status: 200, body: `{"total_items":0,"page_count":0,"items":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form", "list",
		"--search", "nps", "--workspace-id", "ws1",
		"--page", "2", "--page-size", "50",
		"--sort-by", "last_updated_at", "--order-by", "desc", "--is-public")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/forms")
	if req == nil {
		t.Fatal("no GET /forms recorded")
	}
	q := req.Query
	checks := map[string]string{
		"search": "nps", "workspace_id": "ws1", "page": "2",
		"page_size": "50", "sort_by": "last_updated_at", "order_by": "desc",
		"is_public": "true",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestFormListRejectsBadSortBy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "form", "list", "--sort-by", "name")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for bad enum", exit)
	}
	if !strings.Contains(stderr, "sort-by") {
		t.Errorf("stderr = %q, want enum message", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("made %d requests on a parse error, want 0", len(reqs))
	}
}

func TestFormGetPathEscapesID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/abc123": {status: 200, body: `{"id":"abc123","fields":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form", "get", "abc123")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/forms/abc123") == nil {
		t.Fatal("no GET /forms/abc123 recorded")
	}
}

func TestFormCreatePostsDefinitionBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /forms": {status: 201, body: `{"id":"newid","title":"T"}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form", "create", "--definition", `{"title":"T","fields":[]}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodPost, "/forms")
	if req == nil {
		t.Fatal("no POST /forms recorded")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["title"] != "T" {
		t.Errorf("body title = %v, want T", body["title"])
	}
}

func TestFormCreateRejectsInvalidJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "form", "create", "--definition", `{not json`)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for invalid JSON", exit)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want invalid-JSON message", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("made %d requests on invalid JSON, want 0", len(reqs))
	}
}

func TestFormCreateRequiresDefinition(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "form", "create")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 when --definition missing", exit)
	}
	if !strings.Contains(stderr, "definition") {
		t.Errorf("stderr = %q, want required message", stderr)
	}
}

func TestFormUpdatePutsFullDefinition(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /forms/f9": {status: 200, body: `{"id":"f9"}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form", "update", "f9", "--definition", `{"title":"X","fields":[]}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodPut, "/forms/f9") == nil {
		t.Fatal("no PUT /forms/f9 recorded")
	}
}

func TestFormPatchSends204ReceiptOnEmptyBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /forms/f2": {status: 204, body: ``},
	})
	defer srv.Close()

	stdout, _, exit := run(t, srv, "tok", "form", "patch", "f2", "--patch", `[{"op":"replace","path":"/title","value":"New"}]`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodPatch, "/forms/f2")
	if req == nil {
		t.Fatal("no PATCH /forms/f2 recorded")
	}
	if !strings.Contains(stdout, `"patched":true`) {
		t.Errorf("stdout = %q, want patched receipt", stdout)
	}
}

func TestFormDeleteSends204Receipt(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"DELETE /forms/f3": {status: 204, body: ``},
	})
	defer srv.Close()

	stdout, _, exit := run(t, srv, "tok", "form", "delete", "f3")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodDelete, "/forms/f3") == nil {
		t.Fatal("no DELETE /forms/f3 recorded")
	}
	if !strings.Contains(stdout, `"deleted":true`) {
		t.Errorf("stdout = %q, want deleted receipt", stdout)
	}
}
