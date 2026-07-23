package typeform

import (
	"net/http"
	"strings"
	"testing"
)

func TestWorkspaceListPassesParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /workspaces": {status: 200, body: `{"total_items":0,"page_count":0,"items":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "workspace", "list", "--search", "team", "--page", "3", "--page-size", "25")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/workspaces")
	if req == nil {
		t.Fatal("no GET /workspaces recorded")
	}
	if req.Query.Get("search") != "team" || req.Query.Get("page") != "3" || req.Query.Get("page_size") != "25" {
		t.Errorf("query = %v, want search=team page=3 page_size=25", req.Query)
	}
}

func TestWorkspaceGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /workspaces/ws5": {status: 200, body: `{"id":"ws5","name":"W"}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "workspace", "get", "ws5")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/workspaces/ws5") == nil {
		t.Fatal("no GET /workspaces/ws5 recorded")
	}
}

func TestWorkspaceCreatePostsName(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /workspaces": {status: 201, body: `{"id":"wsN","name":"My WS"}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "workspace", "create", "--name", "My WS")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodPost, "/workspaces")
	if req == nil {
		t.Fatal("no POST /workspaces recorded")
	}
	if bodyMap(t, req.Body)["name"] != "My WS" {
		t.Errorf("body = %s, want name=My WS", req.Body)
	}
}

func TestWorkspaceCreateRequiresName(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "workspace", "create")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 without --name", exit)
	}
	if !strings.Contains(strings.ToLower(stderr), "name") {
		t.Errorf("stderr = %q, want required-name message", stderr)
	}
}
