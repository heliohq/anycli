package tasks

import (
	"net/http"
	"strings"
	"testing"
)

func TestListsList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /tasks/v1/users/@me/lists": {http.StatusOK, `{"items":[{"id":"MDg","title":"My Tasks"},{"id":"Rk9P","title":"Groceries"}],"nextPageToken":"NP"}`},
	})
	stdout := f.runOK(t, "lists", "list")
	for _, want := range []string{"MDg", "My Tasks", "Groceries", "next page token: NP"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
	got := f.last(t, "GET", "/tasks/v1/users/@me/lists")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want bearer token", got.Auth)
	}
	if !strings.Contains(got.Query, "maxResults=100") {
		t.Errorf("query = %q, want default maxResults=100", got.Query)
	}
}

func TestListsCreate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /tasks/v1/users/@me/lists": {http.StatusOK, `{"id":"NEW","title":"Trip"}`},
	})
	stdout := f.runOK(t, "lists", "create", "--title", "Trip")
	got := f.last(t, "POST", "/tasks/v1/users/@me/lists")
	if !strings.Contains(string(got.Body), `"title":"Trip"`) {
		t.Errorf("request body = %q, want the title", got.Body)
	}
	if !strings.Contains(stdout, "created task list Trip (NEW)") {
		t.Errorf("human output = %q, want the created summary", stdout)
	}
}

func TestListsUpdate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/users/@me/lists/MDg": {http.StatusOK, `{"id":"MDg","title":"Renamed"}`},
	})
	stdout := f.runOK(t, "lists", "update", "MDg", "--title", "Renamed")
	got := f.last(t, "PATCH", "/tasks/v1/users/@me/lists/MDg")
	if !strings.Contains(string(got.Body), `"title":"Renamed"`) {
		t.Errorf("request body = %q, want the new title", got.Body)
	}
	if !strings.Contains(stdout, "updated task list Renamed (MDg)") {
		t.Errorf("human output = %q, want the updated summary", stdout)
	}
}

func TestListsDelete(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /tasks/v1/users/@me/lists/MDg": {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "lists", "delete", "MDg")
	if f.count("DELETE", "/tasks/v1/users/@me/lists/MDg") != 1 {
		t.Errorf("want exactly one DELETE call")
	}
	if !strings.Contains(stdout, "deleted task list MDg") {
		t.Errorf("human output = %q, want the delete summary", stdout)
	}
}
