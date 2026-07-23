package pinterest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBoardListForwardsPaging(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"bookmark":"NEXT"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "board", "list", "--page-size", "25", "--bookmark", "abc123")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr)
	}
	if got.Path != "/boards" || got.Method != http.MethodGet {
		t.Errorf("request = %s %s, want GET /boards", got.Method, got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("page_size") != "25" || q.Get("bookmark") != "abc123" {
		t.Errorf("query = %q, want page_size=25 bookmark=abc123", got.Query)
	}
	if !strings.Contains(stdout, `"bookmark":"NEXT"`) {
		t.Errorf("stdout = %q, want returned bookmark echoed", stdout)
	}
}

func TestBoardListOmitsUnsetPaging(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "board", "list")
	if got.Query != "" {
		t.Errorf("query = %q, want empty when no paging flags", got.Query)
	}
}

func TestBoardGetEscapesID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"b1"}`, &got)
	defer srv.Close()

	run(t, srv, "board", "get", "b1")
	if got.Path != "/boards/b1" {
		t.Errorf("path = %q, want /boards/b1", got.Path)
	}
}

func TestBoardCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"b9"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "board", "create", "--name", "Recipes", "--description", "yum", "--privacy", "SECRET")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/boards" {
		t.Errorf("request = %s %s, want POST /boards", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Recipes" || body["description"] != "yum" || body["privacy"] != "SECRET" {
		t.Errorf("body = %v, want name/description/privacy set", body)
	}
}

func TestBoardCreateOmitsEmptyOptionalFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	run(t, srv, "board", "create", "--name", "Only Name")
	body := decodeBody(t, got.Body)
	if _, ok := body["description"]; ok {
		t.Error("empty description should be omitted")
	}
	if _, ok := body["privacy"]; ok {
		t.Error("empty privacy should be omitted")
	}
}

func TestBoardDeleteEmptyBodyReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "board", "delete", "b1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodDelete || got.Path != "/boards/b1" {
		t.Errorf("request = %s %s, want DELETE /boards/b1", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"deleted":true`) {
		t.Errorf("stdout = %q, want deletion receipt", stdout)
	}
}

func TestBoardSectionsAndAddSection(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	run(t, srv, "board", "sections", "b1")
	if got.Path != "/boards/b1/sections" || got.Method != http.MethodGet {
		t.Errorf("request = %s %s, want GET /boards/b1/sections", got.Method, got.Path)
	}

	run(t, srv, "board", "add-section", "b1", "--name", "Desserts")
	if got.Path != "/boards/b1/sections" || got.Method != http.MethodPost {
		t.Errorf("request = %s %s, want POST /boards/b1/sections", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Desserts" {
		t.Errorf("body = %v, want name Desserts", body)
	}
}

func TestBoardAddSectionRequiresName(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "board", "add-section", "b1")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "--name is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestBoardPins(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	run(t, srv, "board", "pins", "b1", "--page-size", "10")
	if got.Path != "/boards/b1/pins" {
		t.Errorf("path = %q, want /boards/b1/pins", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("page_size") != "10" {
		t.Errorf("query = %q, want page_size=10", got.Query)
	}
}
