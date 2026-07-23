package outreach

import (
	"net/http"
	"strings"
	"testing"
)

func TestTaskCompletePostsMarkCompleteWithNoteQueryParam(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":{"type":"task","id":"9"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "task", "complete", "9", "--note", "done deal")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/tasks/9/actions/markComplete" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	// The note is a query param (actionParams[completionNote]), not a body.
	if !strings.Contains(got.RawQuery, "actionParams[completionNote]=done+deal") &&
		!strings.Contains(got.RawQuery, "actionParams[completionNote]=done%20deal") {
		t.Fatalf("query = %q, want completionNote param", got.RawQuery)
	}
	if len(got.Body) != 0 {
		t.Fatalf("body = %q, want empty (params go in the query)", got.Body)
	}
}

func TestTaskCompleteWithoutNoteSendsNoParams(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":{"type":"task","id":"9"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "task", "complete", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.RawQuery != "" {
		t.Fatalf("query = %q, want empty", got.RawQuery)
	}
}

func TestTaskSnoozeMapsParamsToActionParams(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":{"type":"task","id":"9"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "task", "snooze", "9", "--param", "snoozeUntil=2026-08-01T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/tasks/9/actions/snooze" {
		t.Fatalf("path = %q", got.Path)
	}
	if !strings.Contains(got.RawQuery, "actionParams[snoozeUntil]=") {
		t.Fatalf("query = %q, want actionParams[snoozeUntil]", got.RawQuery)
	}
}

func TestTaskCreateSetsAttributesAndProspectRelationship(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusCreated, `{"data":{"type":"task","id":"9"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"task", "create", "--due", "2026-08-01T00:00:00Z", "--note", "call back", "--action", "call", "--prospect-id", "1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/tasks" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	for _, want := range []string{`"dueAt":"2026-08-01T00:00:00Z"`, `"note":"call back"`, `"action":"call"`, `"type":"prospect"`, `"id":"1"`} {
		if !strings.Contains(string(got.Body), want) {
			t.Fatalf("body = %s, missing %q", got.Body, want)
		}
	}
}
