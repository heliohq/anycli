package mailerlite

import (
	"net/http"
	"testing"
)

func TestGroupCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "group", "create", "--name", "VIPs"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/groups" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["name"] != "VIPs" {
		t.Errorf("body = %v", body)
	}
}

func TestGroupUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "group", "update", "3", "--name", "Renamed"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/api/groups/3" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestGroupSubscribers(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "group", "subscribers", "3", "--status", "active"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/groups/3/subscribers" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("filter[status]") != "active" {
		t.Errorf("filter[status] = %q", q.Get("filter[status]"))
	}
}

func TestGroupAssign(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "group", "assign", "sub9", "grp4"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/subscribers/sub9/groups/grp4" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestGroupUnassign(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "group", "unassign", "sub9", "grp4"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/api/subscribers/sub9/groups/grp4" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}
