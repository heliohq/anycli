package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTasksList_DefaultListAndFilters(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /tasks/v1/lists/@default/tasks": {http.StatusOK, `{"items":[{"id":"t1","title":"Buy milk","status":"needsAction","due":"2026-07-20T00:00:00.000Z"},{"id":"t2","title":"Ship it","status":"completed"}]}`},
	})
	stdout := f.runOK(t, "list",
		"--due-after", "2026-07-01",
		"--due-before", "2026-07-31",
		"--updated-after", "2026-06-01T00:00:00Z",
		"--show-completed=false",
		"--show-hidden",
		"--show-assigned",
		"--max", "50",
	)
	if !strings.Contains(stdout, "[ ] t1\tBuy milk") || !strings.Contains(stdout, "[x] t2\tShip it") {
		t.Errorf("human output = %q, want status marks + titles", stdout)
	}
	got := f.last(t, "GET", "/tasks/v1/lists/@default/tasks")
	for _, want := range []string{
		"dueMin=2026-07-01T00%3A00%3A00.000Z",
		"dueMax=2026-07-31T00%3A00%3A00.000Z",
		"updatedMin=2026-06-01T00%3A00%3A00Z",
		"showCompleted=false",
		"showHidden=true",
		"showAssigned=true",
		"maxResults=50",
	} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want it to contain %q", got.Query, want)
		}
	}
	if strings.Contains(got.Query, "showDeleted") {
		t.Errorf("query = %q, showDeleted must be absent when the flag is off", got.Query)
	}
}

func TestTasksList_DefaultOmitsShowCompleted(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /tasks/v1/lists/@default/tasks": {http.StatusOK, `{"items":[]}`},
	})
	f.runOK(t, "list")
	got := f.last(t, "GET", "/tasks/v1/lists/@default/tasks")
	if strings.Contains(got.Query, "showCompleted") {
		t.Errorf("query = %q, showCompleted must be omitted when unset (API default true)", got.Query)
	}
	if !strings.Contains(got.Query, "maxResults=20") {
		t.Errorf("query = %q, want default maxResults=20", got.Query)
	}
}

func TestTasksList_ExplicitList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /tasks/v1/lists/Rk9P/tasks": {http.StatusOK, `{"items":[]}`},
	})
	stdout := f.runOK(t, "list", "--list", "Rk9P")
	if !strings.Contains(stdout, "no tasks") {
		t.Errorf("human output = %q, want the empty message", stdout)
	}
}

func TestTasksGet_JSONPassthrough(t *testing.T) {
	body := `{"id":"t1","title":"Buy milk","status":"needsAction"}`
	f := newFixture(t, map[string]route{
		"GET /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "get", "t1", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want raw provider body", stdout)
	}
}

func TestTasksCreate_TitleDueAndPosition(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /tasks/v1/lists/@default/tasks": {http.StatusOK, `{"id":"t9","title":"Call dentist"}`},
	})
	stdout := f.runOK(t, "create", "--title", "Call dentist", "--due", "2026-07-20", "--notes", "afternoon", "--parent", "p1", "--previous", "s1")
	got := f.last(t, "POST", "/tasks/v1/lists/@default/tasks")
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["title"] != "Call dentist" || payload["notes"] != "afternoon" {
		t.Errorf("payload = %v, want title + notes", payload)
	}
	if payload["due"] != "2026-07-20T00:00:00.000Z" {
		t.Errorf("due = %v, want a bare date anchored to UTC midnight", payload["due"])
	}
	if !strings.Contains(got.Query, "parent=p1") || !strings.Contains(got.Query, "previous=s1") {
		t.Errorf("query = %q, want parent + previous", got.Query)
	}
	if !strings.Contains(stdout, "created task Call dentist (t9)") {
		t.Errorf("human output = %q, want created summary", stdout)
	}
}

// TestTasksCreate_DueTimeDroppedVisible: a due with a time part passes through,
// and --json echoes what the API actually stored (time dropped) so the loss is
// visible rather than silently downgraded (design 303 §due-is-a-date).
func TestTasksCreate_DueTimeDroppedVisible(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /tasks/v1/lists/@default/tasks": {http.StatusOK, `{"id":"t9","title":"Remind me","due":"2026-07-20T00:00:00.000Z"}`},
	})
	stdout := f.runOK(t, "create", "--title", "Remind me", "--due", "2026-07-20T15:00:00Z", "--json")
	got := f.last(t, "POST", "/tasks/v1/lists/@default/tasks")
	if !strings.Contains(string(got.Body), `"due":"2026-07-20T15:00:00Z"`) {
		t.Errorf("request body = %q, want the RFC3339 due passed through verbatim", got.Body)
	}
	if !strings.Contains(stdout, `"due":"2026-07-20T00:00:00.000Z"`) {
		t.Errorf("--json output = %q, want the stored (time-dropped) due echoed", stdout)
	}
}

func TestTasksUpdate_PartialFields(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1","title":"New title"}`},
	})
	f.runOK(t, "update", "t1", "--title", "New title")
	got := f.last(t, "PATCH", "/tasks/v1/lists/@default/tasks/t1")
	if !strings.Contains(string(got.Body), `"title":"New title"`) {
		t.Errorf("body = %q, want only the changed title", got.Body)
	}
	if strings.Contains(string(got.Body), "notes") || strings.Contains(string(got.Body), "due") {
		t.Errorf("body = %q, patch must carry only changed fields", got.Body)
	}
}

func TestTasksUpdate_ClearDue(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1","title":"x"}`},
	})
	f.runOK(t, "update", "t1", "--clear-due")
	got := f.last(t, "PATCH", "/tasks/v1/lists/@default/tasks/t1")
	if !strings.Contains(string(got.Body), `"due":null`) {
		t.Errorf("body = %q, want due:null to clear the date", got.Body)
	}
}
