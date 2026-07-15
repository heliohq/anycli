package gmail

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestThreadsList_QueryParams(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/threads": {http.StatusOK, `{"threads":[{"id":"t1","snippet":"hello there"}],"nextPageToken":"npt"}`},
	})
	stdout := f.runOK(t, "threads", "list", "--query", "has:attachment", "--max", "5", "--page-token", "pt")
	got := f.last(t, "GET", "/gmail/v1/users/me/threads")
	for _, param := range []string{"q=has%3Aattachment", "maxResults=5", "pageToken=pt"} {
		if !strings.Contains(got.Query, param) {
			t.Errorf("query = %q, want %q", got.Query, param)
		}
	}
	if !strings.Contains(stdout, "t1") || !strings.Contains(stdout, "hello there") {
		t.Errorf("human output = %q, want id + snippet", stdout)
	}
}

func TestThreadsGet_ExpandsMessagesInOrder(t *testing.T) {
	thread := `{"id":"t9","messages":[` + fullMessage("m1") + `,` + fullMessage("m2") + `]}`
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/threads/t9": {http.StatusOK, thread},
	})
	stdout := f.runOK(t, "threads", "get", "t9")
	got := f.last(t, "GET", "/gmail/v1/users/me/threads/t9")
	if !strings.Contains(got.Query, "format=full") {
		t.Errorf("query = %q, want format=full", got.Query)
	}
	first := strings.Index(stdout, "--- message 1 of 2 ---")
	second := strings.Index(stdout, "--- message 2 of 2 ---")
	if first < 0 || second < 0 || second < first {
		t.Fatalf("human output = %q, want both messages in order", stdout)
	}
	if strings.Count(stdout, "plain body!") != 2 {
		t.Errorf("human output = %q, want each message body expanded", stdout)
	}
	if strings.Count(stdout, "Size:    4321 bytes") != 2 {
		t.Errorf("human output = %q, want the size estimate on each message", stdout)
	}
}

func TestThreadsGetJSON_PerMessageSizeEstimate(t *testing.T) {
	thread := `{"id":"t9","messages":[` + fullMessage("m1") + `,` + fullMessage("m2") + `]}`
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/threads/t9": {http.StatusOK, thread},
	})
	stdout := f.runOK(t, "threads", "get", "t9", "--json")
	var parsed struct {
		ID       string        `json:"id"`
		Messages []messageView `json:"messages"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(parsed.Messages))
	}
	for i, m := range parsed.Messages {
		if m.SizeEstimate != 4321 {
			t.Errorf("message %d size estimate = %d, want 4321", i, m.SizeEstimate)
		}
	}
	if !strings.Contains(stdout, `"size_estimate":4321`) {
		t.Errorf("--json output = %q, want the size_estimate field per message", stdout)
	}
}
