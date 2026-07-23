package brevo

import (
	"net/http"
	"testing"
)

func TestSenderLs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"senders":[{"id":1,"email":"n@myco.com","active":true}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "sender", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/senders" {
		t.Errorf("request = %s %s, want GET /senders", got.Method, got.Path)
	}
	if stdout == "" {
		t.Error("want senders JSON on stdout")
	}
}

func TestSenderLs_DomainFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"senders":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "sender", "ls", "--domain", "myco.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if q := parseQuery(t, got.Query); q.Get("domain") != "myco.com" {
		t.Errorf("domain = %q, want myco.com", q.Get("domain"))
	}
}
