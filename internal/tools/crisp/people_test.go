package crisp

import (
	"net/http"
	"net/url"
	"testing"
)

// TestPeopleList proves the profiles page-suffixed path.
func TestPeopleList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "people", "list", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/website/wid-1/people/profiles/1" {
		t.Errorf("got %s %s, want GET /website/wid-1/people/profiles/1", got.Method, got.Path)
	}
}

// TestPeopleListSearch proves --search maps to the search_text query param.
func TestPeopleListSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "people", "list", "--website", "wid-1", "--page", "2", "--search", "jane doe")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Path != "/website/wid-1/people/profiles/2" {
		t.Errorf("path = %q, want page 2", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("search_text") != "jane doe" {
		t.Errorf("search_text = %q, want 'jane doe'", q.Get("search_text"))
	}
}

// TestPeopleGet proves the singular people profile path.
func TestPeopleGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{"people_id":"p1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "people", "get", "--people", "p1", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/website/wid-1/people/profile/p1" {
		t.Errorf("got %s %s, want GET /website/wid-1/people/profile/p1", got.Method, got.Path)
	}
}

// TestPeopleCreate proves the POST profile body (email + person.nickname).
func TestPeopleCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{"people_id":"p9"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "people", "create", "--email", "new@example.com", "--nickname", "New Person", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/website/wid-1/people/profile" {
		t.Errorf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "new@example.com" {
		t.Errorf("email = %v", body["email"])
	}
	person, ok := body["person"].(map[string]any)
	if !ok || person["nickname"] != "New Person" {
		t.Errorf("person = %v, want nickname 'New Person'", body["person"])
	}
}

// TestPeopleCreateEmailOnly proves --nickname is optional (no person block).
func TestPeopleCreateEmailOnly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":{"people_id":"p9"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "people", "create", "--email", "a@b.com", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["person"]; ok {
		t.Errorf("person block present with no nickname: %v", body)
	}
}
