package attio

import (
	"net/http"
	"testing"
)

func TestNoteCreateMarkdownVsPlaintextFormat(t *testing.T) {
	cases := []struct {
		name       string
		flag, val  string
		wantFormat string
	}{
		{"markdown", "--markdown", "# Hi", "markdown"},
		{"plaintext", "--plaintext", "hi", "plaintext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{"POST /v2/notes": okData(`{"id":{"note_id":"n-1"}}`)})
			defer srv.Close()

			_, errStr, exit := runService(t, srv, "note", "create",
				"--parent", "people:r-1", "--title", "Call", tc.flag, tc.val)
			if exit != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
			}
			data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/notes").Body)
			if data["parent_object"] != "people" || data["parent_record_id"] != "r-1" {
				t.Errorf("parent = %v/%v, want people/r-1", data["parent_object"], data["parent_record_id"])
			}
			if data["format"] != tc.wantFormat {
				t.Errorf("format = %v, want %s", data["format"], tc.wantFormat)
			}
			if data["content"] != tc.val {
				t.Errorf("content = %v, want %q", data["content"], tc.val)
			}
			if data["title"] != "Call" {
				t.Errorf("title = %v, want Call", data["title"])
			}
		})
	}
}

func TestNoteCreateRequiresExactlyOneBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	// Neither markdown nor plaintext.
	if _, _, exit := runService(t, srv, "note", "create", "--parent", "people:r-1", "--title", "x"); exit != 2 {
		t.Errorf("no body flag: exit = %d, want 2", exit)
	}
	// Both.
	if _, _, exit := runService(t, srv, "note", "create", "--parent", "people:r-1", "--title", "x",
		"--markdown", "a", "--plaintext", "b"); exit != 2 {
		t.Errorf("both body flags: exit = %d, want 2", exit)
	}
	if len(reqs) != 0 {
		t.Errorf("invalid note create must not reach API, got %d requests", len(reqs))
	}
}

func TestNoteListFilterQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v2/notes": okData(`[]`)})
	defer srv.Close()

	_, _, exit := runService(t, srv, "note", "list", "--record", "companies:c-1", "--limit", "5")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/v2/notes")
	if req.Query.Get("parent_object") != "companies" || req.Query.Get("parent_record_id") != "c-1" {
		t.Errorf("note list filter params wrong: %v", req.Query)
	}
	if req.Query.Get("limit") != "5" {
		t.Errorf("limit param = %q, want 5", req.Query.Get("limit"))
	}
}

func TestTaskCreateRequiredFieldsAlwaysSent(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /v2/tasks": okData(`{"id":{"task_id":"t-1"}}`)})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "task", "create", "--content", "Follow up")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/tasks").Body)
	if data["content"] != "Follow up" {
		t.Errorf("content = %v", data["content"])
	}
	if data["format"] != "plaintext" {
		t.Errorf("format = %v, want plaintext", data["format"])
	}
	if data["is_completed"] != false {
		t.Errorf("is_completed = %v, want false", data["is_completed"])
	}
	if _, present := data["deadline_at"]; !present {
		t.Error("deadline_at must be present (null) even when --deadline omitted")
	}
	if data["deadline_at"] != nil {
		t.Errorf("deadline_at = %v, want null when --deadline omitted", data["deadline_at"])
	}
	if lr, ok := data["linked_records"].([]any); !ok || len(lr) != 0 {
		t.Errorf("linked_records = %v, want empty array", data["linked_records"])
	}
	if as, ok := data["assignees"].([]any); !ok || len(as) != 0 {
		t.Errorf("assignees = %v, want empty array", data["assignees"])
	}
}

func TestTaskCreateWithDeadlineAssigneeRecord(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /v2/tasks": okData(`{"id":{"task_id":"t-1"}}`)})
	defer srv.Close()

	_, _, exit := runService(t, srv, "task", "create", "--content", "x",
		"--deadline", "2026-01-01T15:00:00Z", "--assignee", "m-1", "--record", "people:r-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/tasks").Body)
	if data["deadline_at"] != "2026-01-01T15:00:00Z" {
		t.Errorf("deadline_at = %v", data["deadline_at"])
	}
	as := data["assignees"].([]any)
	a0 := as[0].(map[string]any)
	if a0["referenced_actor_type"] != "workspace-member" || a0["referenced_actor_id"] != "m-1" {
		t.Errorf("assignee = %v", a0)
	}
	lr := data["linked_records"].([]any)
	l0 := lr[0].(map[string]any)
	if l0["target_object"] != "people" || l0["target_record_id"] != "r-1" {
		t.Errorf("linked_record = %v", l0)
	}
}

func TestTaskUpdateRequiresAtLeastOneField(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"PATCH /v2/tasks/t-1": okData(`{"id":{"task_id":"t-1"}}`)})
	defer srv.Close()

	if _, _, exit := runService(t, srv, "task", "update", "t-1"); exit != 2 {
		t.Errorf("empty task update: exit = %d, want 2", exit)
	}
	if len(reqs) != 0 {
		t.Error("empty task update must not reach API")
	}
	if _, _, exit := runService(t, srv, "task", "update", "t-1", "--completed", "true"); exit != 0 {
		t.Fatalf("task update --completed exit != 0")
	}
	data := dataMap(t, findReq(reqs, http.MethodPatch, "/v2/tasks/t-1").Body)
	if data["is_completed"] != true {
		t.Errorf("is_completed = %v, want true", data["is_completed"])
	}
}

func TestCommentCreateDefaultsAuthorFromSelf(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self":      {status: http.StatusOK, body: `{"workspace_id":"ws-1","authorized_by_workspace_member_id":"m-self"}`},
		"POST /v2/comments": okData(`{"id":{"comment_id":"c-1"}}`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "comment", "create", "--record", "people:r-1", "--content", "hi")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/comments").Body)
	if data["format"] != "plaintext" {
		t.Errorf("format = %v, want plaintext", data["format"])
	}
	author := data["author"].(map[string]any)
	if author["type"] != "workspace-member" || author["id"] != "m-self" {
		t.Errorf("author = %v, want default m-self", author)
	}
	rec := data["record"].(map[string]any)
	if rec["object"] != "people" || rec["record_id"] != "r-1" {
		t.Errorf("record target = %v", rec)
	}
}

func TestCommentCreateAuthorOverrideSkipsSelf(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/comments": okData(`{"id":{"comment_id":"c-1"}}`),
	})
	defer srv.Close()

	_, _, exit := runService(t, srv, "comment", "create", "--thread", "th-1", "--content", "hi", "--author", "m-override")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/v2/self") != nil {
		t.Error("with --author, /v2/self must not be called")
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/comments").Body)
	if data["thread_id"] != "th-1" {
		t.Errorf("thread_id = %v, want th-1", data["thread_id"])
	}
	author := data["author"].(map[string]any)
	if author["id"] != "m-override" {
		t.Errorf("author id = %v, want m-override", author["id"])
	}
}

func TestCommentCreateFailsFastWhenNoMemberID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self": {status: http.StatusOK, body: `{"workspace_id":"ws-1"}`},
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "comment", "create", "--record", "people:r-1", "--content", "hi")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (usage: no resolvable author)", exit)
	}
	if findReq(reqs, http.MethodPost, "/v2/comments") != nil {
		t.Error("must not POST a comment without a resolvable author")
	}
	if errStr == "" {
		t.Error("expected an explanatory error requiring --author")
	}
}

func TestCommentCreateThreadRecordMutualExclusion(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	// Neither.
	if _, _, exit := runService(t, srv, "comment", "create", "--content", "hi", "--author", "m-1"); exit != 2 {
		t.Errorf("neither target: exit = %d, want 2", exit)
	}
	// Both.
	if _, _, exit := runService(t, srv, "comment", "create", "--content", "hi", "--author", "m-1",
		"--thread", "th-1", "--record", "people:r-1"); exit != 2 {
		t.Errorf("both targets: exit = %d, want 2", exit)
	}
}

func TestThreadListRecordScoping(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v2/threads": okData(`[]`)})
	defer srv.Close()

	_, _, exit := runService(t, srv, "thread", "list", "--record", "companies:c-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/v2/threads")
	if req.Query.Get("object") != "companies" || req.Query.Get("record_id") != "c-1" {
		t.Errorf("thread list scoping wrong: %v", req.Query)
	}
}

func TestBadRecordRefIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	if _, _, exit := runService(t, srv, "note", "list", "--record", "no-colon"); exit != 2 {
		t.Errorf("malformed --record: exit = %d, want 2", exit)
	}
	if len(reqs) != 0 {
		t.Error("malformed --record must not reach API")
	}
}
