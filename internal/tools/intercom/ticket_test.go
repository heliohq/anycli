package intercom

import (
	"net/http"
	"testing"
)

func TestTicketCreate_ContactsAndAttributes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"ticket","id":"tk1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "create",
		"--ticket-type-id", "18", "--contact-id", "c1", "--contact-id", "c2",
		"--attributes-json", `{"_default_title_":"Broken","priority":"high"}`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/tickets" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["ticket_type_id"] != "18" {
		t.Errorf("ticket_type_id = %v", body["ticket_type_id"])
	}
	contacts, ok := body["contacts"].([]any)
	if !ok || len(contacts) != 2 {
		t.Fatalf("contacts = %v, want 2", body["contacts"])
	}
	first := contacts[0].(map[string]any)
	if first["id"] != "c1" {
		t.Errorf("contacts[0] = %v, want {id:c1}", first)
	}
	if _, ok := body["ticket_attributes"].(map[string]any); !ok {
		t.Errorf("ticket_attributes = %v, want object", body["ticket_attributes"])
	}
}

func TestTicketUpdate_AssignmentAndState(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"ticket"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "update", "--id", "tk1",
		"--state", "in_progress", "--assignee-id", "530165", "--admin-id", "1", "--open")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/tickets/tk1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["state"] != "in_progress" {
		t.Errorf("state = %v", body["state"])
	}
	if body["open"] != true {
		t.Errorf("open = %v, want true", body["open"])
	}
	assignment, ok := body["assignment"].(map[string]any)
	if !ok || assignment["assignee_id"] != "530165" || assignment["admin_id"] != "1" {
		t.Errorf("assignment = %v", body["assignment"])
	}
}

func TestTicketReply_AdminComment(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"ticket_part"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "reply", "--id", "tk1", "--body", "on it", "--admin-id", "1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/tickets/tk1/reply" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "comment" || body["type"] != "admin" || body["admin_id"] != "1" {
		t.Errorf("body = %v", body)
	}
}

func TestTicketTypeList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"ticket_type.list"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "type-list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/ticket_types" {
		t.Errorf("request = %s %s, want GET /ticket_types", got.Method, got.Path)
	}
}
