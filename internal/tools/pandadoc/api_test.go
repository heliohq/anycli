package pandadoc

import (
	"strings"
	"testing"
)

func TestAPI_RawPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "api", "GET", "/documents", "--query", "count=1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/public/v1/documents" {
		t.Errorf("request = %s %s, want GET /public/v1/documents", got.Method, got.Path)
	}
	if got.Query.Get("count") != "1" {
		t.Errorf("query count = %q, want 1", got.Query.Get("count"))
	}
}

func TestAPI_PathPrefixNormalization(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	// A caller may pass the full public/v1-prefixed path; it must not double up.
	exit, _, _ := run(t, srv, "api", "GET", "/public/v1/members/current")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/members/current" {
		t.Errorf("path = %q, want /public/v1/members/current", got.Path)
	}
}

func TestAPI_BodyPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "api", "POST", "/contacts", "--body", `{"email":"x@y.com"}`)
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	b := bodyMap(t, got.Body)
	if b["email"] != "x@y.com" {
		t.Errorf("body = %v", b)
	}
	if !strings.Contains(got.ContentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}
}
