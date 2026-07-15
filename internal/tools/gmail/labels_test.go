package gmail

import (
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
