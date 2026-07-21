package salesloft

import (
	"net/http"
	"testing"
)

func TestTaskList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/tasks": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "task", "list", "--filter", "current_state=scheduled")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/tasks")
	if req == nil || req.Query.Get("current_state") != "scheduled" {
		t.Fatalf("expected GET /v2/tasks with current_state filter, got %+v", req)
	}
}

func TestTaskCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/tasks": {status: http.StatusOK, body: `{"data":{"id":1}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "task", "create",
		"--subject", "Follow up", "--task-type", "call", "--person-id", "5",
		"--due-date", "2026-08-01", "--current-state", "scheduled")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPost, "/v2/tasks")
	if req == nil {
		t.Fatal("expected POST /v2/tasks")
	}
	body := bodyMap(t, req.Body)
	if body["subject"] != "Follow up" || body["task_type"] != "call" {
		t.Errorf("task fields = %v", body)
	}
	if body["person_id"] != float64(5) {
		t.Errorf("person_id = %v, want numeric 5", body["person_id"])
	}
	if body["current_state"] != "scheduled" {
		t.Errorf("current_state = %v", body["current_state"])
	}
}

func TestTaskUpdate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/tasks/8": {status: http.StatusOK, body: `{"data":{"id":8}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "task", "update", "--id", "8", "--current-state", "completed")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPut, "/v2/tasks/8")
	if req == nil {
		t.Fatal("expected PUT /v2/tasks/8")
	}
	body := bodyMap(t, req.Body)
	if body["current_state"] != "completed" {
		t.Errorf("current_state = %v", body["current_state"])
	}
	// subject was not set, must be omitted.
	if _, ok := body["subject"]; ok {
		t.Errorf("subject should be omitted when unset; body = %v", body)
	}
}
