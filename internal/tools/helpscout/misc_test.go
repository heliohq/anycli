package helpscout

import (
	"net/http"
	"strings"
	"testing"
)

func TestCustomerListAndCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"_embedded":{"customers":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "customer", "list", "--query", "email:a@b.com", "--page", "2")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/customers" {
		t.Errorf("path = %s", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("query") != "email:a@b.com" || q.Get("page") != "2" {
		t.Errorf("query = %s", got.Query)
	}
}

func TestCustomerCreate_EmailBecomesEmailsArray(t *testing.T) {
	var got capturedRequest
	srv := newHeaderServer(t, http.StatusCreated, ``, map[string]string{"Resource-Id": "77"}, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "customer", "create", "--first-name", "Ada", "--email", "ada@x.com")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	body := decodeBody(t, got.Body)
	if body["firstName"] != "Ada" {
		t.Errorf("firstName = %v", body["firstName"])
	}
	emails, _ := body["emails"].([]any)
	if len(emails) != 1 {
		t.Fatalf("emails = %v", body["emails"])
	}
	em, _ := emails[0].(map[string]any)
	if em["value"] != "ada@x.com" || em["type"] != "work" {
		t.Errorf("email entry = %v", em)
	}
	if decodeBody(t, []byte(stdout))["id"] != "77" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

func TestCustomerCreate_RequiresAField(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "customer", "create")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call with no fields")
	}
}

func TestInboxFolders(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"_embedded":{"folders":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "inbox", "folders", "12")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/mailboxes/12/folders" {
		t.Errorf("path = %s", got.Path)
	}
}

func TestSavedReplyListRequiresInbox(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "saved-reply", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call without --inbox")
	}
}

func TestSavedReplyList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"_embedded":{"saved-replies":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "saved-reply", "list", "--inbox", "12")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/mailboxes/12/saved-replies" {
		t.Errorf("path = %s", got.Path)
	}
}

func TestUserMe(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1,"email":"me@x.com"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "me")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/users/me" {
		t.Errorf("path = %s", got.Path)
	}
	if !strings.Contains(stdout, `"email":"me@x.com"`) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestTagList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"_embedded":{"tags":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "tag", "list")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/tags" {
		t.Errorf("path = %s", got.Path)
	}
}
