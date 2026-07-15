package gmail

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLabelsList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/labels": {http.StatusOK, `{"labels":[{"id":"INBOX","name":"INBOX","type":"system"},{"id":"Label_1","name":"Receipts","type":"user"}]}`},
	})
	stdout := f.runOK(t, "labels", "list")
	if !strings.Contains(stdout, "Receipts") || !strings.Contains(stdout, "Label_1") {
		t.Errorf("human output = %q, want label ids + names", stdout)
	}
}

func TestLabelsGet_Counters(t *testing.T) {
	body := `{"id":"INBOX","name":"INBOX","type":"system","messagesTotal":120,"messagesUnread":4,"threadsTotal":98,"threadsUnread":3}`
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/labels/INBOX": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "labels", "get", "INBOX")
	for _, want := range []string{
		"Id:              INBOX",
		"Type:            system",
		"MessagesTotal:   120",
		"MessagesUnread:  4",
		"ThreadsTotal:    98",
		"ThreadsUnread:   3",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want it to contain %q", stdout, want)
		}
	}

	stdout = f.runOK(t, "labels", "get", "INBOX", "--json")
	var l struct {
		ID             string `json:"id"`
		MessagesUnread int64  `json:"messagesUnread"`
		ThreadsUnread  int64  `json:"threadsUnread"`
	}
	if err := json.Unmarshal([]byte(stdout), &l); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
	if l.ID != "INBOX" || l.MessagesUnread != 4 || l.ThreadsUnread != 3 {
		t.Errorf("--json label = %+v, want the INBOX counters", l)
	}
}

func TestLabelsCreate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/labels": {http.StatusOK, `{"id":"Label_9","name":"Follow up","type":"user"}`},
	})
	stdout := f.runOK(t, "labels", "create", "Follow up")
	got := f.last(t, "POST", "/gmail/v1/users/me/labels")
	if !strings.Contains(string(got.Body), `"name":"Follow up"`) {
		t.Errorf("request body = %q, want the label name", got.Body)
	}
	if !strings.Contains(stdout, "created label Follow up (Label_9)") {
		t.Errorf("human output = %q, want the created summary", stdout)
	}
}
