package gmail

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twoAttachmentMessage is a format=full fixture with two attachments sharing
// a filename.
func twoAttachmentMessage() string {
	m := map[string]any{
		"id":       "m1",
		"threadId": "t1",
		"payload": map[string]any{
			"mimeType": "multipart/mixed",
			"headers":  []map[string]string{{"name": "Subject", "value": "files"}},
			"body":     map[string]any{"size": 0},
			"parts": []map[string]any{
				{
					"partId":   "0",
					"mimeType": "text/plain",
					"body":     map[string]any{"size": 2, "data": b64url("hi")},
				},
				{
					"partId":   "1",
					"mimeType": "text/csv",
					"filename": "data.csv",
					"body":     map[string]any{"size": 9, "attachmentId": "att-a"},
				},
				{
					"partId":   "2",
					"mimeType": "text/csv",
					"filename": "data.csv",
					"body":     map[string]any{"size": 6, "attachmentId": "att-b"},
				},
			},
		},
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func attachmentData(content string) string {
	out, _ := json.Marshal(map[string]any{"size": len(content), "data": b64url(content)})
	return string(out)
}

func TestAttachments_DownloadsAllPartsByDefault(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":                   {http.StatusOK, twoAttachmentMessage()},
		"GET /gmail/v1/users/me/messages/m1/attachments/att-a": {http.StatusOK, attachmentData("col1,col2\n")},
		"GET /gmail/v1/users/me/messages/m1/attachments/att-b": {http.StatusOK, attachmentData("other\n")},
	})
	stdout := f.runOK(t, "messages", "attachments", "m1", "--save", dir)

	first, err := os.ReadFile(filepath.Join(dir, "data.csv"))
	if err != nil {
		t.Fatalf("first attachment not written: %v", err)
	}
	if string(first) != "col1,col2\n" {
		t.Errorf("first attachment = %q, want the decoded csv", first)
	}
	second, err := os.ReadFile(filepath.Join(dir, "data-1.csv"))
	if err != nil {
		t.Fatalf("second attachment (deduped name) not written: %v", err)
	}
	if string(second) != "other\n" {
		t.Errorf("second attachment = %q, want the decoded csv", second)
	}
	if !strings.Contains(stdout, "data.csv") || !strings.Contains(stdout, "data-1.csv") {
		t.Errorf("human output = %q, want both saved paths", stdout)
	}
}

func TestAttachments_SelectByIndex(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":                   {http.StatusOK, twoAttachmentMessage()},
		"GET /gmail/v1/users/me/messages/m1/attachments/att-b": {http.StatusOK, attachmentData("only-b")},
	})
	stdout := f.runOK(t, "messages", "attachments", "m1", "--index", "2", "--save", dir, "--json")
	if len(f.requests) != 2 {
		t.Fatalf("saw %d requests, want message fetch + one attachment fetch", len(f.requests))
	}
	var saved []savedAttachment
	if err := json.Unmarshal([]byte(stdout), &saved); err != nil {
		t.Fatalf("--json output not a saved list: %v", err)
	}
	if len(saved) != 1 || saved[0].AttachmentID != "att-b" || saved[0].Size != 6 {
		t.Errorf("saved = %+v, want only att-b (the 2nd inventory entry)", saved)
	}
	if _, err := os.Stat(saved[0].Path); err != nil {
		t.Errorf("saved path %s not on disk: %v", saved[0].Path, err)
	}
}

func TestAttachments_SelectByName(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1":                   {http.StatusOK, twoAttachmentMessage()},
		"GET /gmail/v1/users/me/messages/m1/attachments/att-a": {http.StatusOK, attachmentData("aaa")},
		"GET /gmail/v1/users/me/messages/m1/attachments/att-b": {http.StatusOK, attachmentData("bbb")},
	})
	// Both fixture parts are named data.csv: an exact-name match selects both.
	stdout := f.runOK(t, "messages", "attachments", "m1", "--name", "data.csv", "--save", dir, "--json")
	if len(f.requests) != 3 {
		t.Fatalf("saw %d requests, want message fetch + both matching attachment fetches", len(f.requests))
	}
	var saved []savedAttachment
	if err := json.Unmarshal([]byte(stdout), &saved); err != nil {
		t.Fatalf("--json output not a saved list: %v", err)
	}
	if len(saved) != 2 || saved[0].AttachmentID != "att-a" || saved[1].AttachmentID != "att-b" {
		t.Errorf("saved = %+v, want both data.csv parts in inventory order", saved)
	}
}

func TestAttachments_UnknownNameFails(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1": {http.StatusOK, twoAttachmentMessage()},
	})
	result, _, stderr := f.run(t, "messages", "attachments", "m1", "--name", "nope.pdf")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, `no attachment named "nope.pdf"`) {
		t.Errorf("stderr = %q, want the unknown-name error", stderr)
	}
}

func TestAttachments_IndexOutOfRangeFails(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m1": {http.StatusOK, twoAttachmentMessage()},
	})
	result, _, stderr := f.run(t, "messages", "attachments", "m1", "--index", "3")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "index 3 out of range") {
		t.Errorf("stderr = %q, want the out-of-range error", stderr)
	}
	if len(f.requests) != 1 {
		t.Errorf("saw %d requests, want only the message fetch (no downloads)", len(f.requests))
	}
}

// The --attachment-id flag was removed: Gmail regenerates attachmentIds on
// every messages.get, so ids from a previous get are a trap.
func TestAttachments_AttachmentIDFlagRemoved(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "messages", "attachments", "m1", "--attachment-id", "att-b")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "unknown flag: --attachment-id") {
		t.Errorf("stderr = %q, want the unknown-flag error", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("flag failure must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestAttachments_NoAttachments(t *testing.T) {
	noAtt := `{"id":"m2","threadId":"t2","payload":{"mimeType":"text/plain","headers":[],"body":{"size":2,"data":"` + b64url("hi") + `"}}}`
	f := newFixture(t, map[string]route{
		"GET /gmail/v1/users/me/messages/m2": {http.StatusOK, noAtt},
	})
	stdout := f.runOK(t, "messages", "attachments", "m2")
	if !strings.Contains(stdout, "no attachments") {
		t.Errorf("human output = %q, want the empty notice", stdout)
	}
}
