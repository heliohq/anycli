package acuity

import (
	"net/http"
	"testing"
)

func TestClientList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"firstName":"Jane"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "client", "list", "--search", "jane")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/clients" {
		t.Fatalf("request = %s %s, want GET /clients", got.Method, got.Path)
	}
	assertQuery(t, parseQuery(t, got.Query), "search", "jane")
}

func TestClientCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"firstName":"Jane"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"client", "create",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--email", "jane@example.com",
		"--phone", "555-1234",
		"--notes", "walk-in",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/clients" {
		t.Fatalf("request = %s %s, want POST /clients", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["firstName"] != "Jane" || body["lastName"] != "Doe" {
		t.Errorf("name fields wrong: %v", body)
	}
	if body["email"] != "jane@example.com" || body["phone"] != "555-1234" || body["notes"] != "walk-in" {
		t.Errorf("optional fields wrong: %v", body)
	}
}

func TestClientUpdateKeysOnNameViaQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"firstName":"Jane"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"client", "update",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--email", "new@example.com",
		"--notes", "updated",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/clients" {
		t.Fatalf("request = %s %s, want PUT /clients", got.Method, got.Path)
	}
	// Identity is carried in the query string.
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "firstName", "Jane")
	assertQuery(t, q, "lastName", "Doe")
	// Updated values live in the body (with the identity echoed, as Acuity requires).
	body := decodeBody(t, got.Body)
	if body["firstName"] != "Jane" || body["lastName"] != "Doe" {
		t.Errorf("body must echo identity name: %v", body)
	}
	if body["email"] != "new@example.com" || body["notes"] != "updated" {
		t.Errorf("update fields wrong: %v", body)
	}
}

func TestClientDeleteKeysOnNameViaQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"client", "delete",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--phone", "555-1234",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/clients" {
		t.Fatalf("request = %s %s, want DELETE /clients", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "firstName", "Jane")
	assertQuery(t, q, "lastName", "Doe")
	assertQuery(t, q, "phone", "555-1234")
}

func TestClientUpdateRequiresName(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "client", "update", "--email", "x@example.com")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing name", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("usage error must not reach the API")
	}
}
