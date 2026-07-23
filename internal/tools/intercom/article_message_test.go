package intercom

import (
	"net/http"
	"testing"
)

func TestArticleSearch_PhraseGetQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"article_search_response"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "article", "search", "--phrase", "refund policy", "--state", "published")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/articles/search" {
		t.Errorf("request = %s %s, want GET /articles/search", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("phrase") != "refund policy" || q.Get("state") != "published" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestArticleCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"article","id":"a1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "article", "create",
		"--title", "How to refund", "--author-id", "991", "--body", "<p>Steps</p>", "--state", "draft")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/articles" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["title"] != "How to refund" || body["author_id"] != "991" || body["state"] != "draft" {
		t.Errorf("body = %v", body)
	}
}

func TestArticleCollectionList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"list"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "article", "collection-list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/help_center/collections" {
		t.Errorf("path = %q, want /help_center/collections", got.Path)
	}
}

func TestMessageSend_InappToContact(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"admin_conversation"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send",
		"--message-type", "inapp", "--body", "hi there", "--from-admin-id", "1", "--to-contact-id", "c1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/messages" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["message_type"] != "inapp" || body["body"] != "hi there" {
		t.Errorf("body = %v", body)
	}
	from := body["from"].(map[string]any)
	if from["type"] != "admin" || from["id"] != "1" {
		t.Errorf("from = %v", from)
	}
	to := body["to"].(map[string]any)
	if to["type"] != "user" || to["id"] != "c1" {
		t.Errorf("to = %v", to)
	}
}

func TestMessageSend_EmailSubjectAndAutoAdmin(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/me":       {status: http.StatusOK, response: `{"type":"admin","id":"55"}`},
		"/messages": {status: http.StatusOK, response: `{"type":"admin_conversation"}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send",
		"--message-type", "email", "--subject", "Welcome", "--body", "hello", "--to-email", "a@b.com")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, ok := captured["/me"]; !ok {
		t.Fatal("expected GET /me to auto-resolve the sending admin")
	}
	body := decodeBody(t, captured["/messages"].Body)
	if body["subject"] != "Welcome" {
		t.Errorf("subject = %v, want Welcome (email type)", body["subject"])
	}
	from := body["from"].(map[string]any)
	if from["id"] != "55" {
		t.Errorf("from.id = %v, want auto-resolved 55", from["id"])
	}
	to := body["to"].(map[string]any)
	if to["email"] != "a@b.com" {
		t.Errorf("to = %v, want email target", to)
	}
}

func TestTagCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"tag","id":"t9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "tag", "create", "--name", "billing")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/tags" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["name"] != "billing" {
		t.Errorf("body = %v", body)
	}
}

func TestCompanyUpsert(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"company","id":"co1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "upsert", "--company-id", "acme-1", "--name", "Acme")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/companies" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["company_id"] != "acme-1" || body["name"] != "Acme" {
		t.Errorf("body = %v", body)
	}
}
