package gmail

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func b64url(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// fullMessage is a format=full fixture: multipart with a text body, an html
// body, and one attachment.
func fullMessage(id string) string {
	m := map[string]any{
		"id":       id,
		"threadId": "t9",
		"labelIds": []string{"INBOX", "UNREAD"},
		"payload": map[string]any{
			"mimeType": "multipart/mixed",
			"headers": []map[string]string{
				{"name": "From", "value": "Ada Lovelace <ada@example.com>"},
				{"name": "To", "value": "me@example.com, Bob <bob@example.com>"},
				{"name": "Cc", "value": "carol@example.com"},
				{"name": "Subject", "value": "Quarterly numbers"},
				{"name": "Date", "value": "Tue, 14 Jul 2026 10:00:00 -0700"},
				{"name": "Message-ID", "value": "<orig-123@mail.example.com>"},
				{"name": "References", "value": "<root-1@mail.example.com>"},
			},
			"body": map[string]any{"size": 0},
			"parts": []map[string]any{
				{
					"partId":   "0",
					"mimeType": "text/plain",
					"filename": "",
					"body":     map[string]any{"size": 11, "data": b64url("plain body!")},
				},
				{
					"partId":   "1",
					"mimeType": "text/html",
					"filename": "",
					"body":     map[string]any{"size": 20, "data": b64url("<b>html body</b>")},
				},
				{
					"partId":   "2",
					"mimeType": "application/pdf",
					"filename": "report.pdf",
					"body":     map[string]any{"size": 3, "attachmentId": "att-1"},
				},
			},
		},
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func TestMessagesList_QueryParams(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages": {http.StatusOK, `{"messages":[{"id":"m1","threadId":"t1"}],"nextPageToken":"npt-7"}`},
	})
	stdout := f.runOK(t, "messages", "list",
		"--query", "is:unread newer_than:7d", "--label", "INBOX", "--max", "3", "--page-token", "pt-1")
	got := f.last(t, "GET", "/gmail/v1/users/me/messages")
	for _, param := range []string{"q=is%3Aunread+newer_than%3A7d", "labelIds=INBOX", "maxResults=3", "pageToken=pt-1"} {
		if !strings.Contains(got.Query, param) {
			t.Errorf("query = %q, want %q", got.Query, param)
		}
	}
	if !strings.Contains(stdout, "m1") || !strings.Contains(stdout, "next page token: npt-7") {
		t.Errorf("human output = %q, want ids + next page token", stdout)
	}
}

func TestMessagesList_JSONPassthrough(t *testing.T) {
	body := `{"messages":[{"id":"m1","threadId":"t1"}]}`
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "messages", "list", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestMessagesGet_BodyAndAttachmentInventory(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1": {http.StatusOK, fullMessage("m1")},
	})
	stdout := f.runOK(t, "messages", "get", "m1")
	got := f.last(t, "GET", "/gmail/v1/users/me/messages/m1")
	if !strings.Contains(got.Query, "format=full") {
		t.Errorf("query = %q, want format=full", got.Query)
	}
	for _, want := range []string{"plain body!", "Quarterly numbers", "att-1", "report.pdf", "3 bytes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want it to contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "html body") {
		t.Errorf("human output = %q, default body must be text, not html", stdout)
	}
}

func TestMessagesGet_HTMLBodyAndJSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1": {http.StatusOK, fullMessage("m1")},
	})
	stdout := f.runOK(t, "messages", "get", "m1", "--body", "html", "--json")
	var view messageView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("--json output is not a message view: %v", err)
	}
	if view.BodyType != "html" || view.Body != "<b>html body</b>" {
		t.Errorf("body = (%s, %q), want the html part", view.BodyType, view.Body)
	}
	if len(view.Attachments) != 1 || view.Attachments[0].AttachmentID != "att-1" || view.Attachments[0].Filename != "report.pdf" || view.Attachments[0].Size != 3 {
		t.Errorf("attachments = %+v, want the report.pdf inventory entry", view.Attachments)
	}
	if view.Headers["Subject"] != "Quarterly numbers" {
		t.Errorf("headers = %v, want the Subject header", view.Headers)
	}
}

func TestMessagesModify_SingleUsesPerMessageEndpoint(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/m1/modify": {http.StatusOK, `{"id":"m1","labelIds":["STARRED"]}`},
	})
	f.runOK(t, "messages", "modify", "m1", "--add-label", "STARRED", "--mark-read")
	got := f.last(t, "POST", "/gmail/v1/users/me/messages/m1/modify")
	var payload struct {
		AddLabelIDs    []string `json:"addLabelIds"`
		RemoveLabelIDs []string `json:"removeLabelIds"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if len(payload.AddLabelIDs) != 1 || payload.AddLabelIDs[0] != "STARRED" {
		t.Errorf("addLabelIds = %v, want [STARRED]", payload.AddLabelIDs)
	}
	if len(payload.RemoveLabelIDs) != 1 || payload.RemoveLabelIDs[0] != "UNREAD" {
		t.Errorf("removeLabelIds = %v, want [UNREAD] from --mark-read", payload.RemoveLabelIDs)
	}
}

func TestMessagesModify_MultipleUseBatchModify(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/batchModify": {http.StatusNoContent, ""},
	})
	stdout := f.runOK(t, "messages", "modify", "m1", "m2", "m3", "--archive")
	if len(f.requests) != 1 {
		t.Fatalf("saw %d requests, want exactly one batchModify call", len(f.requests))
	}
	got := f.last(t, "POST", "/gmail/v1/users/me/messages/batchModify")
	var payload struct {
		IDs            []string `json:"ids"`
		RemoveLabelIDs []string `json:"removeLabelIds"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if len(payload.IDs) != 3 || payload.IDs[0] != "m1" || payload.IDs[2] != "m3" {
		t.Errorf("ids = %v, want [m1 m2 m3]", payload.IDs)
	}
	if len(payload.RemoveLabelIDs) != 1 || payload.RemoveLabelIDs[0] != "INBOX" {
		t.Errorf("removeLabelIds = %v, want [INBOX] from --archive", payload.RemoveLabelIDs)
	}
	if !strings.Contains(stdout, "modified 3 messages") {
		t.Errorf("human output = %q, want the batch summary", stdout)
	}
}

func TestMessagesTrash_MultipleIDs(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/m1/trash": {http.StatusOK, `{"id":"m1"}`},
		"POST /gmail/v1/users/me/messages/m2/trash": {http.StatusOK, `{"id":"m2"}`},
	})
	stdout := f.runOK(t, "messages", "trash", "m1", "m2", "--json")
	if len(f.requests) != 2 {
		t.Fatalf("saw %d requests, want one trash call per id", len(f.requests))
	}
	if !strings.Contains(stdout, `"status":"trashed"`) {
		t.Errorf("--json output = %q, want the trashed status", stdout)
	}
}

func TestMessagesUntrash(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /gmail/v1/users/me/messages/m1/untrash": {http.StatusOK, `{"id":"m1"}`},
	})
	stdout := f.runOK(t, "messages", "untrash", "m1")
	if !strings.Contains(stdout, "untrashed 1 message(s)") {
		t.Errorf("human output = %q, want the untrashed summary", stdout)
	}
}
