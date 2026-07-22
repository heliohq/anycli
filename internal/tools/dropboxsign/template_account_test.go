package dropboxsign

import (
	"strings"
	"testing"
)

func TestTemplateList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"templates":[],"list_info":{"page":1}}`, &got)
	defer srv.Close()
	exit, stdout, _ := run(t, srv, "template", "list", "--page-size", "3")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/template/list" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
	if !strings.Contains(got.Query, "page_size=3") {
		t.Fatalf("query = %q", got.Query)
	}
	contains(t, stdout, `"templates"`, "template list stdout")
}

func TestTemplateGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"template":{"template_id":"tmpl-1"}}`, &got)
	defer srv.Close()
	exit, _, _ := run(t, srv, "template", "get", "tmpl-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/template/tmpl-1" {
		t.Fatalf("path = %s", got.Path)
	}
}

func TestAccountGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"account":{"account_id":"acc-1","email_address":"me@example.com"}}`, &got)
	defer srv.Close()
	exit, stdout, _ := run(t, srv, "account", "get")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/account" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
	if got.Query != "" {
		t.Fatalf("account get should send no query params, got %q", got.Query)
	}
	contains(t, stdout, `"email_address":"me@example.com"`, "account stdout")
}
