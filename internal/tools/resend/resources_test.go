package resend

import (
	"net/http"
	"testing"
)

func TestDomainList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"d1","name":"example.com"}]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/domains" {
		t.Errorf("request = %s %s, want GET /domains", got.Method, got.Path)
	}
}

func TestDomainGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"d1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "get", "d1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/domains/d1" {
		t.Errorf("request = %s %s, want GET /domains/d1", got.Method, got.Path)
	}
}

func TestDomainCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"d1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "create", "--name", "example.com", "--region", "us-east-1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/domains" {
		t.Errorf("request = %s %s, want POST /domains", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "example.com" || body["region"] != "us-east-1" {
		t.Errorf("body = %v", body)
	}
}

func TestDomainVerify(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"d1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "verify", "d1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/domains/d1/verify" {
		t.Errorf("request = %s %s, want POST /domains/d1/verify", got.Method, got.Path)
	}
}

func TestDomainDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"d1","deleted":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "delete", "d1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/domains/d1" {
		t.Errorf("request = %s %s, want DELETE /domains/d1", got.Method, got.Path)
	}
}

func TestAudienceCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"a1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "audience", "create", "--name", "Registered Users")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/audiences" {
		t.Errorf("request = %s %s, want POST /audiences", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["name"] != "Registered Users" {
		t.Errorf("name = %v", body["name"])
	}
}

func TestAudienceListGetDelete(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/audiences":    {status: http.StatusOK, response: `{"data":[]}`},
		"/audiences/a1": {status: http.StatusOK, response: `{"id":"a1"}`},
	}, captured)
	defer srv.Close()

	if code, _, _ := run(t, srv, "audience", "list"); code != 0 {
		t.Fatalf("list exit = %d", code)
	}
	if code, _, _ := run(t, srv, "audience", "get", "a1"); code != 0 {
		t.Fatalf("get exit = %d", code)
	}
	if code, _, _ := run(t, srv, "audience", "delete", "a1"); code != 0 {
		t.Fatalf("delete exit = %d", code)
	}
	if captured["/audiences/a1"].Method != http.MethodDelete {
		t.Errorf("last method on /audiences/a1 = %s, want DELETE", captured["/audiences/a1"].Method)
	}
}

func TestContactCreate_NestedUnderAudience(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"c1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--audience", "a1",
		"--email", "user@d.com", "--first-name", "Ada", "--unsubscribed")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/audiences/a1/contacts" {
		t.Errorf("request = %s %s, want POST /audiences/a1/contacts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "user@d.com" || body["first_name"] != "Ada" {
		t.Errorf("body = %v", body)
	}
	if body["unsubscribed"] != true {
		t.Errorf("unsubscribed = %v, want true", body["unsubscribed"])
	}
}

func TestContactUpdateDelete_ByID(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/audiences/a1/contacts/c1": {status: http.StatusOK, response: `{"id":"c1"}`},
	}, captured)
	defer srv.Close()

	if code, _, _ := run(t, srv, "contact", "update", "c1", "--audience", "a1", "--unsubscribed"); code != 0 {
		t.Fatalf("update exit = %d", code)
	}
	if captured["/audiences/a1/contacts/c1"].Method != http.MethodPatch {
		t.Errorf("update method = %s, want PATCH", captured["/audiences/a1/contacts/c1"].Method)
	}
	if code, _, _ := run(t, srv, "contact", "delete", "c1", "--audience", "a1"); code != 0 {
		t.Fatalf("delete exit = %d", code)
	}
	if captured["/audiences/a1/contacts/c1"].Method != http.MethodDelete {
		t.Errorf("delete method = %s, want DELETE", captured["/audiences/a1/contacts/c1"].Method)
	}
}

func TestBroadcastCreateAndSend(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/broadcasts":         {status: http.StatusOK, response: `{"id":"b1"}`},
		"/broadcasts/b1/send": {status: http.StatusOK, response: `{"id":"b1"}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "broadcast", "create",
		"--audience", "a1", "--from", "a@example.com", "--subject", "News", "--html", "<p>hi</p>")
	if code != 0 {
		t.Fatalf("create exit = %d", code)
	}
	body := decodeBody(t, captured["/broadcasts"].Body)
	if body["audience_id"] != "a1" || body["from"] != "a@example.com" {
		t.Errorf("body = %v", body)
	}

	code, _, _ = run(t, srv, "broadcast", "send", "b1", "--scheduled-at", "in 1 hour")
	if code != 0 {
		t.Fatalf("send exit = %d", code)
	}
	if captured["/broadcasts/b1/send"].Method != http.MethodPost {
		t.Errorf("send method = %s, want POST", captured["/broadcasts/b1/send"].Method)
	}
	if sb := decodeBody(t, captured["/broadcasts/b1/send"].Body); sb["scheduled_at"] != "in 1 hour" {
		t.Errorf("send scheduled_at = %v", sb["scheduled_at"])
	}
}
