package hubspot

import (
	"net/http"
	"testing"
)

func TestNoteCreateDefaultsTimestampAndAssociates(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"n1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "note", "create", "--body", "Called the customer", "--contact", "581751", "--deal", "42")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/crm/v3/objects/notes" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	props := body["properties"].(map[string]any)
	if props["hs_note_body"] != "Called the customer" {
		t.Fatalf("hs_note_body = %v", props["hs_note_body"])
	}
	if ts, ok := props["hs_timestamp"].(string); !ok || ts == "" {
		t.Fatalf("hs_timestamp should default, got %#v", props["hs_timestamp"])
	}
	assocs := body["associations"].([]any)
	if len(assocs) != 2 {
		t.Fatalf("associations = %#v", assocs)
	}
	// contact → typeId 202
	a0 := assocs[0].(map[string]any)
	if a0["to"].(map[string]any)["id"] != "581751" {
		t.Fatalf("assoc[0] target = %#v", a0["to"])
	}
	typ0 := a0["types"].([]any)[0].(map[string]any)
	if typ0["associationCategory"] != "HUBSPOT_DEFINED" || typ0["associationTypeId"].(float64) != 202 {
		t.Fatalf("assoc[0] type = %#v", typ0)
	}
	// deal → typeId 214
	a1 := assocs[1].(map[string]any)
	typ1 := a1["types"].([]any)[0].(map[string]any)
	if typ1["associationTypeId"].(float64) != 214 {
		t.Fatalf("assoc[1] typeId = %v", typ1["associationTypeId"])
	}
}

func TestNoteCreateHonorsExplicitTimestamp(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"n1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "note", "create", "--body", "x", "--timestamp", "2021-11-12T15:48:22Z")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	props := decodeBody(t, got.Body)["properties"].(map[string]any)
	if props["hs_timestamp"] != "2021-11-12T15:48:22Z" {
		t.Fatalf("hs_timestamp = %v", props["hs_timestamp"])
	}
}

func TestTaskCreateMapsFieldsAndAssociates(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"t1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"task", "create",
		"--subject", "Follow up",
		"--body", "Send the proposal",
		"--due", "2026-08-01T09:00:00Z",
		"--owner", "555",
		"--status", "NOT_STARTED",
		"--priority", "HIGH",
		"--company", "900",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	props := decodeBody(t, got.Body)["properties"].(map[string]any)
	if props["hs_task_subject"] != "Follow up" || props["hs_task_body"] != "Send the proposal" {
		t.Fatalf("task props = %#v", props)
	}
	if props["hs_timestamp"] != "2026-08-01T09:00:00Z" {
		t.Fatalf("hs_timestamp = %v", props["hs_timestamp"])
	}
	if props["hubspot_owner_id"] != "555" || props["hs_task_status"] != "NOT_STARTED" || props["hs_task_priority"] != "HIGH" {
		t.Fatalf("task props = %#v", props)
	}
	// company → task typeId 192
	assocs := decodeBody(t, got.Body)["associations"].([]any)
	typ := assocs[0].(map[string]any)["types"].([]any)[0].(map[string]any)
	if typ["associationTypeId"].(float64) != 192 {
		t.Fatalf("task→company typeId = %v", typ["associationTypeId"])
	}
}

func TestTaskCompleteSetsStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"t1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "task", "complete", "t1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodPatch || got.Path != "/crm/v3/objects/tasks/t1" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	props := decodeBody(t, got.Body)["properties"].(map[string]any)
	if props["hs_task_status"] != "COMPLETED" {
		t.Fatalf("status = %v", props["hs_task_status"])
	}
}
