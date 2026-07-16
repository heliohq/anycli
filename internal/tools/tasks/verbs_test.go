package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestComplete_PatchesStatus(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1","status":"completed"}`},
	})
	stdout := f.runOK(t, "complete", "t1")
	got := f.last(t, "PATCH", "/tasks/v1/lists/@default/tasks/t1")
	if !strings.Contains(string(got.Body), `"status":"completed"`) {
		t.Errorf("body = %q, want status=completed", got.Body)
	}
	if !strings.Contains(stdout, "completed t1") {
		t.Errorf("human output = %q, want completed summary", stdout)
	}
}

func TestReopen_PatchesStatus(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1","status":"needsAction"}`},
	})
	f.runOK(t, "reopen", "t1")
	got := f.last(t, "PATCH", "/tasks/v1/lists/@default/tasks/t1")
	if !strings.Contains(string(got.Body), `"status":"needsAction"`) {
		t.Errorf("body = %q, want status=needsAction", got.Body)
	}
}

// TestComplete_MultipleSerialWithPerIDFailure: several ids are patched serially,
// a mid-list failure does not abort the rest, and the failing id is reported
// while the exit code goes non-zero.
func TestComplete_MultipleSerialWithPerIDFailure(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1","status":"completed"}`},
		"PATCH /tasks/v1/lists/@default/tasks/t2": {http.StatusNotFound, `{"error":{"status":"NOT_FOUND","message":"no such task"}}`},
		"PATCH /tasks/v1/lists/@default/tasks/t3": {http.StatusOK, `{"id":"t3","status":"completed"}`},
	})
	result, stdout, stderr := f.run(t, "complete", "t1", "t2", "t3")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 on partial failure", result.ExitCode)
	}
	if f.count("PATCH", "/tasks/v1/lists/@default/tasks/t3") != 1 {
		t.Error("t3 must still be attempted after t2 failed")
	}
	if !strings.Contains(stdout, "completed t1") || !strings.Contains(stdout, "completed t3") {
		t.Errorf("stdout = %q, want the successful ids", stdout)
	}
	if !strings.Contains(stderr, "failed t2") || !strings.Contains(stderr, "no such task") {
		t.Errorf("stderr = %q, want the failed id reported", stderr)
	}
}

func TestComplete_MultipleFromOneArg(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1"}`},
		"PATCH /tasks/v1/lists/@default/tasks/t2": {http.StatusOK, `{"id":"t2"}`},
	})
	f.runOK(t, "complete", "t1 t2")
	if f.count("PATCH", "/tasks/v1/lists/@default/tasks/t1") != 1 || f.count("PATCH", "/tasks/v1/lists/@default/tasks/t2") != 1 {
		t.Error("whitespace-joined ids must split into separate patches")
	}
}

func TestComplete_JSONResults(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /tasks/v1/lists/@default/tasks/t1": {http.StatusOK, `{"id":"t1"}`},
		"PATCH /tasks/v1/lists/@default/tasks/t2": {http.StatusOK, `{"id":"t2"}`},
	})
	stdout := f.runOK(t, "complete", "t1", "t2", "--json")
	var out struct {
		Results []perIDOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("--json output not valid: %v", err)
	}
	if len(out.Results) != 2 || out.Results[0].Status != "completed" {
		t.Errorf("results = %+v, want two completed rows", out.Results)
	}
}

func TestMove_ToList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /tasks/v1/lists/@default/tasks/t1/move": {http.StatusOK, `{"id":"t1"}`},
	})
	f.runOK(t, "move", "t1", "--to-list", "Rk9P", "--parent", "p1", "--previous", "s1")
	got := f.last(t, "POST", "/tasks/v1/lists/@default/tasks/t1/move")
	for _, want := range []string{"destinationTasklist=Rk9P", "parent=p1", "previous=s1"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want %q", got.Query, want)
		}
	}
}

func TestClear_HidesCompleted(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /tasks/v1/lists/@default/clear": {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "clear")
	if f.count("POST", "/tasks/v1/lists/@default/clear") != 1 {
		t.Error("want one clear call")
	}
	if !strings.Contains(stdout, "cleared completed tasks in @default") {
		t.Errorf("human output = %q, want the clear summary", stdout)
	}
}

func TestDelete_Serial(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /tasks/v1/lists/@default/tasks/t1": {http.StatusNoContent, ``},
		"DELETE /tasks/v1/lists/@default/tasks/t2": {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "delete", "t1", "t2")
	if f.count("DELETE", "/tasks/v1/lists/@default/tasks/t1") != 1 || f.count("DELETE", "/tasks/v1/lists/@default/tasks/t2") != 1 {
		t.Error("both ids must be deleted")
	}
	if !strings.Contains(stdout, "deleted t1") || !strings.Contains(stdout, "deleted t2") {
		t.Errorf("human output = %q, want both delete summaries", stdout)
	}
}

func TestDelete_401RejectsCredential(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /tasks/v1/lists/@default/tasks/t1": {http.StatusUnauthorized, `{"error":{"status":"UNAUTHENTICATED","message":"invalid token"}}`},
	})
	result, _, stderr := f.run(t, "delete", "t1")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 UNAUTHENTICATED must reject the credential")
	}
	if !strings.Contains(stderr, "invalid token") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
}
