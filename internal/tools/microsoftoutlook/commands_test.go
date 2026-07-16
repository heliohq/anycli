package microsoftoutlook

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessagesList_HumanJSONAndFolder(t *testing.T) {
	listBody := `{"value":[{"id":"m1","subject":"Hello","isRead":false,"receivedDateTime":"2026-07-16T10:00:00Z","from":{"emailAddress":{"name":"Bob","address":"bob@x.com"}}}],"@odata.nextLink":"` + "PLACEHOLDER" + `"}`
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/messages":                   {http.StatusOK, listBody},
		"GET /v1.0/me/mailFolders/inbox/messages": {http.StatusOK, `{"value":[]}`},
	})
	stdout := f.runOK(t, "messages", "list", "--search", "hello", "--filter", "isRead eq false", "--max", "5")
	if !strings.Contains(stdout, "m1") || !strings.Contains(stdout, "Hello") || !strings.Contains(stdout, "[unread]") {
		t.Errorf("human output = %q, want id/subject/unread", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/messages")
	if !strings.Contains(got.Query, "%24search=%22hello%22") {
		t.Errorf("query = %q, want $search=\"hello\"", got.Query)
	}
	if !strings.Contains(got.Query, "%24filter=isRead+eq+false") {
		t.Errorf("query = %q, want $filter passthrough", got.Query)
	}
	if !strings.Contains(got.Query, "%24top=5") {
		t.Errorf("query = %q, want $top=5", got.Query)
	}

	// --folder scopes the path to the folder's messages collection.
	f.runOK(t, "messages", "list", "--folder", "inbox")
	f.last(t, "GET", "/v1.0/me/mailFolders/inbox/messages")

	// --json passes the raw body through.
	stdout = f.runOK(t, "messages", "list", "--json")
	if !json.Valid([]byte(strings.TrimSpace(stdout))) {
		t.Errorf("--json output is not valid JSON: %q", stdout)
	}
}

func TestMessagesGet_BodyPreferenceAndAttachments(t *testing.T) {
	msg := `{"id":"m1","subject":"Report","from":{"emailAddress":{"address":"a@b.c"}},"body":{"contentType":"text","content":"hi there"},"isRead":true,"attachments":[{"id":"att1","name":"f.pdf","contentType":"application/pdf","size":1024}]}`
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/messages/m1": {http.StatusOK, msg},
	})
	stdout := f.runOK(t, "messages", "get", "m1", "--body", "text")
	if !strings.Contains(stdout, "hi there") || !strings.Contains(stdout, "f.pdf") {
		t.Errorf("human output = %q, want body + attachment inventory", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/messages/m1")
	if got.Prefer != `outlook.body-content-type="text"` {
		t.Errorf("Prefer = %q, want the text body preference", got.Prefer)
	}
	if !strings.Contains(got.Query, "%24expand=attachments") {
		t.Errorf("query = %q, want $expand=attachments", got.Query)
	}

	// --headers adds the internetMessageHeaders $select.
	f.runOK(t, "messages", "get", "m1", "--headers")
	got = f.last(t, "GET", "/v1.0/me/messages/m1")
	if !strings.Contains(got.Query, "internetMessageHeaders") {
		t.Errorf("query = %q, want internetMessageHeaders select with --headers", got.Query)
	}
}

func TestMessagesMove_SingleAndBatch(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/messages/m1/move": {http.StatusCreated, `{"id":"m1"}`},
		"POST /v1.0/$batch":              {http.StatusOK, `{"responses":[{"id":"1","status":201},{"id":"2","status":201}]}`},
	})
	// Single id → direct move.
	f.runOK(t, "messages", "move", "m1", "--folder", "archive")
	got := f.last(t, "POST", "/v1.0/me/messages/m1/move")
	if !strings.Contains(string(got.Body), `"destinationId":"archive"`) {
		t.Errorf("move body = %q, want destinationId", string(got.Body))
	}

	// Multiple ids → $batch.
	stdout := f.runOK(t, "messages", "move", "m1", "m2", "--folder", "archive")
	if !strings.Contains(stdout, "moved 2 message(s)") {
		t.Errorf("output = %q, want batch move summary", stdout)
	}
	batch := f.last(t, "POST", "/v1.0/$batch")
	if !strings.Contains(string(batch.Body), `"/me/messages/m1/move"`) || !strings.Contains(string(batch.Body), `"/me/messages/m2/move"`) {
		t.Errorf("batch body = %q, want both move sub-requests", string(batch.Body))
	}
}

func TestMessagesMark_ReadAndFlag(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /v1.0/me/messages/m1": {http.StatusOK, `{"id":"m1","isRead":true}`},
	})
	f.runOK(t, "messages", "mark", "m1", "--read", "--flag")
	got := f.last(t, "PATCH", "/v1.0/me/messages/m1")
	if !strings.Contains(string(got.Body), `"isRead":true`) {
		t.Errorf("mark body = %q, want isRead true", string(got.Body))
	}
	if !strings.Contains(string(got.Body), `"flagStatus":"flagged"`) {
		t.Errorf("mark body = %q, want flag flagged", string(got.Body))
	}
}

func TestMessagesSend(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/sendMail": {http.StatusAccepted, ``},
	})
	stdout := f.runOK(t, "messages", "send", "--to", "a@b.c,d@e.f", "--subject", "Hi", "--body", "hello", "--html")
	if !strings.Contains(stdout, "sent message") {
		t.Errorf("output = %q, want sent confirmation", stdout)
	}
	got := f.last(t, "POST", "/v1.0/me/sendMail")
	var payload struct {
		Message struct {
			Subject string `json:"subject"`
			Body    struct {
				ContentType string `json:"contentType"`
			} `json:"body"`
			ToRecipients []struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
			} `json:"toRecipients"`
		} `json:"message"`
		SaveToSentItems bool `json:"saveToSentItems"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("send payload decode: %v", err)
	}
	if payload.Message.Subject != "Hi" || payload.Message.Body.ContentType != "html" {
		t.Errorf("payload = %+v, want subject Hi and html body", payload)
	}
	if len(payload.Message.ToRecipients) != 2 {
		t.Errorf("recipients = %d, want 2", len(payload.Message.ToRecipients))
	}
	if !payload.SaveToSentItems {
		t.Error("saveToSentItems should be true")
	}
}

func TestMessagesReply_CreateReplyThenSend(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/messages/m1/createReplyAll": {http.StatusCreated, `{"id":"draft9"}`},
		"POST /v1.0/me/messages/draft9/send":       {http.StatusAccepted, ``},
	})
	stdout := f.runOK(t, "messages", "reply", "m1", "--body", "thanks", "--all")
	if !strings.Contains(stdout, "sent reply") {
		t.Errorf("output = %q, want reply confirmation", stdout)
	}
	created := f.last(t, "POST", "/v1.0/me/messages/m1/createReplyAll")
	if !strings.Contains(string(created.Body), `"comment":"thanks"`) {
		t.Errorf("createReplyAll body = %q, want comment", string(created.Body))
	}
	f.last(t, "POST", "/v1.0/me/messages/draft9/send")
}

func TestMessagesForward(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/messages/m1/createForward": {http.StatusCreated, `{"id":"draft5"}`},
		"POST /v1.0/me/messages/draft5/send":      {http.StatusAccepted, ``},
	})
	f.runOK(t, "messages", "forward", "m1", "--to", "x@y.z", "--body", "fyi")
	created := f.last(t, "POST", "/v1.0/me/messages/m1/createForward")
	if !strings.Contains(string(created.Body), `"x@y.z"`) || !strings.Contains(string(created.Body), `"comment":"fyi"`) {
		t.Errorf("createForward body = %q, want recipient + comment", string(created.Body))
	}
}

func TestFoldersList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/mailFolders": {http.StatusOK, `{"value":[{"id":"inbox","displayName":"Inbox","unreadItemCount":3,"totalItemCount":42}]}`},
	})
	stdout := f.runOK(t, "folders", "list")
	if !strings.Contains(stdout, "Inbox") || !strings.Contains(stdout, "unread 3") {
		t.Errorf("output = %q, want folder + unread count", stdout)
	}
}

func TestDrafts_CreateListSendDelete(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/messages":                   {http.StatusCreated, `{"id":"d1"}`},
		"GET /v1.0/me/mailFolders/drafts/messages": {http.StatusOK, `{"value":[{"id":"d1","subject":"WIP","toRecipients":[{"emailAddress":{"address":"a@b.c"}}]}]}`},
		"POST /v1.0/me/messages/d1/send":           {http.StatusAccepted, ``},
		"DELETE /v1.0/me/messages/d1":              {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "drafts", "create", "--to", "a@b.c", "--subject", "WIP", "--body", "draft body")
	if !strings.Contains(stdout, "created draft d1") {
		t.Errorf("output = %q, want created draft", stdout)
	}
	created := f.last(t, "POST", "/v1.0/me/messages")
	if !strings.Contains(string(created.Body), `"subject":"WIP"`) {
		t.Errorf("draft create body = %q, want subject", string(created.Body))
	}

	stdout = f.runOK(t, "drafts", "list")
	if !strings.Contains(stdout, "d1") || !strings.Contains(stdout, "WIP") {
		t.Errorf("list output = %q, want draft", stdout)
	}

	stdout = f.runOK(t, "drafts", "send", "d1")
	if !strings.Contains(stdout, "sent draft") {
		t.Errorf("send output = %q, want sent", stdout)
	}

	stdout = f.runOK(t, "drafts", "delete", "d1")
	if !strings.Contains(stdout, "deleted draft d1") {
		t.Errorf("delete output = %q, want deleted", stdout)
	}
}

func TestAttachmentsDownload(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/messages/m1/attachments": {http.StatusOK, `{"value":[{"@odata.type":"#microsoft.graph.fileAttachment","id":"att1","name":"note.txt","contentType":"text/plain","size":5,"contentBytes":"aGVsbG8="}]}`},
	})
	stdout := f.runOK(t, "messages", "attachments", "m1", "--save", dir)
	if !strings.Contains(stdout, "note.txt") {
		t.Errorf("output = %q, want saved attachment", stdout)
	}
	data, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil {
		t.Fatalf("read saved attachment: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("attachment content = %q, want decoded base64 'hello'", string(data))
	}
}

func TestCredentialRejected_401RealCommand(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/mailFolders": {http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"token expired"}}`},
	})
	result, _, _ := f.run(t, "folders", "list")
	if !result.CredentialRejected {
		t.Error("401 InvalidAuthenticationToken must reject the credential")
	}
}
