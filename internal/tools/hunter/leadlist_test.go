package hunter

import (
	"net/http"
	"testing"
)

func TestLeadListList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"leads_lists":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "lead-list", "list", "--limit", "20")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/leads_lists" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("limit") != "20" {
		t.Errorf("limit = %q", q.Get("limit"))
	}
}

func TestLeadListGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":3}}`, &got)
	defer srv.Close()

	run(t, srv, "lead-list", "get", "--id", "3")
	if got.Path != "/leads_lists/3" {
		t.Errorf("path = %s", got.Path)
	}
}

func TestLeadListCreate_NameRequired(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":3,"name":"Prospects"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "lead-list", "create", "--name", "Prospects", "--team-id", "11")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/leads_lists" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Prospects" || body["team_id"] != "11" {
		t.Errorf("body = %v", body)
	}
}

func TestLeadListCreate_MissingNameIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "lead-list", "create")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Error("should not call API without required --name")
	}
}

func TestLeadListUpdateAndDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":3}}`, &got)
	defer srv.Close()

	run(t, srv, "lead-list", "update", "--id", "3", "--name", "Renamed")
	if got.Method != http.MethodPut || got.Path != "/leads_lists/3" {
		t.Errorf("update request = %s %s", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["name"] != "Renamed" {
		t.Errorf("name = %v", body["name"])
	}

	var got2 capturedRequest
	srv2 := newServer(t, http.StatusNoContent, ``, &got2)
	defer srv2.Close()
	code, stdout, _ := run(t, srv2, "lead-list", "delete", "--id", "3")
	if code != 0 {
		t.Fatalf("delete exit = %d", code)
	}
	if got2.Method != http.MethodDelete || got2.Path != "/leads_lists/3" {
		t.Errorf("delete request = %s %s", got2.Method, got2.Path)
	}
	if stdout == "" {
		t.Error("want deletion receipt")
	}
}
