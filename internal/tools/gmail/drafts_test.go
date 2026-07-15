package gmail

import (
	"net/http"
	"strings"
	"testing"
)

func TestDraftsCreate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/drafts": {http.StatusOK, `{"id":"d1","message":{"id":"m1","threadId":"t1"}}`},
	})
	stdout := f.runOK(t, "drafts", "create", "--to", "a@b.c", "--subject", "Draft subject", "--body", "draft body")
	got := f.last(t, "POST", "/gmail/v1/users/me/drafts")
	mime, _ := decodeRaw(t, got.Body)
	if !strings.Contains(mime, "Subject: Draft subject\r\n") || !strings.Contains(mime, "draft body") {
		t.Errorf("MIME = %q, want subject + body", mime)
	}
	if !strings.Contains(stdout, "created draft d1") {
		t.Errorf("human output = %q, want the created summary", stdout)
	}
}

func TestDraftsUpdate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /gmail/v1/users/me/drafts/d1": {http.StatusOK, `{"id":"d1","message":{"id":"m2"}}`},
	})
	f.runOK(t, "drafts", "update", "d1", "--to", "a@b.c", "--subject", "v2", "--body", "new body")
	got := f.last(t, "PUT", "/gmail/v1/users/me/drafts/d1")
	mime, _ := decodeRaw(t, got.Body)
	if !strings.Contains(mime, "Subject: v2\r\n") || !strings.Contains(mime, "new body") {
		t.Errorf("MIME = %q, want the replacement content", mime)
	}
}

func TestDraftsList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/drafts": {http.StatusOK, `{"drafts":[{"id":"d1","message":{"id":"m1"}}]}`},
	})
	stdout := f.runOK(t, "drafts", "list", "--max", "4")
	got := f.last(t, "GET", "/gmail/v1/users/me/drafts")
	if !strings.Contains(got.Query, "maxResults=4") {
		t.Errorf("query = %q, want maxResults=4", got.Query)
	}
	if !strings.Contains(stdout, "d1") {
		t.Errorf("human output = %q, want the draft id", stdout)
	}
}

func TestDraftsGet(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/drafts/d1": {http.StatusOK, `{"id":"d1","message":` + fullMessage("m1") + `}`},
	})
	stdout := f.runOK(t, "drafts", "get", "d1")
	if !strings.Contains(stdout, "Draft:   d1") || !strings.Contains(stdout, "plain body!") {
		t.Errorf("human output = %q, want draft id + message body", stdout)
	}
}

func TestDraftsSend(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/drafts/send": {http.StatusOK, `{"id":"m5","threadId":"t5"}`},
	})
	stdout := f.runOK(t, "drafts", "send", "d1")
	got := f.last(t, "POST", "/gmail/v1/users/me/drafts/send")
	if !strings.Contains(string(got.Body), `"id":"d1"`) {
		t.Errorf("request body = %q, want the draft id", got.Body)
	}
	if !strings.Contains(stdout, "sent draft d1 as message m5") {
		t.Errorf("human output = %q, want the sent summary", stdout)
	}
}

func TestDraftsDelete(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /gmail/v1/users/me/drafts/d1": {http.StatusNoContent, ""},
	})
	stdout := f.runOK(t, "drafts", "delete", "d1", "--json")
	if !strings.Contains(stdout, `"status":"deleted"`) {
		t.Errorf("--json output = %q, want the deleted status", stdout)
	}
}
