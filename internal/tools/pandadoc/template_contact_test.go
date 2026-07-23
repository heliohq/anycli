package pandadoc

import (
	"strings"
	"testing"
)

func TestTemplateList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"results":[{"id":"t-1","name":"NDA Template"}]}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "template", "list", "--q", "nda", "--count", "5", "--page", "1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/templates" {
		t.Errorf("path = %q, want /public/v1/templates", got.Path)
	}
	if got.Query.Get("q") != "nda" || got.Query.Get("count") != "5" || got.Query.Get("page") != "1" {
		t.Errorf("query = %v", got.Query)
	}
	if !strings.Contains(stdout, "t-1") || !strings.Contains(stdout, "NDA Template") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestTemplateDetails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"t-1","name":"NDA","roles":[{"name":"Client"}]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "template", "details", "t-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/templates/t-1/details" {
		t.Errorf("path = %q, want /public/v1/templates/t-1/details", got.Path)
	}
}

func TestContactList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"id":"c-1","email":"a@b.com"}]`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "contact", "list", "--email", "a@b.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/contacts" {
		t.Errorf("path = %q, want /public/v1/contacts", got.Path)
	}
	if got.Query.Get("email") != "a@b.com" {
		t.Errorf("query email = %q", got.Query.Get("email"))
	}
	if !strings.Contains(stdout, "c-1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestContactCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":"c-9","email":"z@z.com"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contact", "create",
		"--email", "z@z.com", "--first", "Zoe", "--last", "Z", "--company", "ZCorp", "--phone", "+1555")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "POST" || got.Path != "/public/v1/contacts" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	b := bodyMap(t, got.Body)
	if b["email"] != "z@z.com" || b["first_name"] != "Zoe" || b["last_name"] != "Z" ||
		b["company"] != "ZCorp" || b["phone"] != "+1555" {
		t.Errorf("contact body = %v", b)
	}
}
