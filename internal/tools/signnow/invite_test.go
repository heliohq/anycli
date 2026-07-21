package signnow

import (
	"net/http"
	"testing"
)

func TestInviteSend_FieldInvite_ResolvesSender(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user":                  {http.StatusOK, `{"id":"u1","primary_email":"me@acme.com"}`},
		"POST /document/doc1/invite": {http.StatusOK, `{"status":"success"}`},
	})
	defer srv.Close()

	to := `[{"email":"signer@x.com","role":"Signer 1","order":1}]`
	res, _, stderr := runSN(t, srv, "invite", "send", "doc1", "--to", to, "--subject", "Please sign")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/document/doc1/invite")
	if got == nil {
		t.Fatalf("no invite request recorded")
	}
	body := bodyMap(t, got.Body)
	if body["from"] != "me@acme.com" {
		t.Errorf("from = %v, want the resolved primary email", body["from"])
	}
	if _, ok := body["to"].([]any); !ok {
		t.Errorf("field invite must send a to array, got %v", body["to"])
	}
	if body["subject"] != "Please sign" {
		t.Errorf("subject = %v, want Please sign", body["subject"])
	}
}

func TestInviteSend_ExplicitFrom_SkipsUserLookup(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /document/doc1/invite": {http.StatusOK, `{"status":"success"}`},
	})
	defer srv.Close()

	to := `[{"email":"signer@x.com","role":"Signer 1"}]`
	res, _, stderr := runSN(t, srv, "invite", "send", "doc1", "--to", to, "--from", "boss@acme.com")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	if findReq(reqs, http.MethodGet, "/user") != nil {
		t.Errorf("explicit --from must skip the GET /user sender lookup")
	}
	body := bodyMap(t, findReq(reqs, http.MethodPost, "/document/doc1/invite").Body)
	if body["from"] != "boss@acme.com" {
		t.Errorf("from = %v, want boss@acme.com", body["from"])
	}
}

func TestInviteSend_FreeForm_Email(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /document/doc1/invite": {http.StatusOK, `{"id":"inv1"}`},
	})
	defer srv.Close()

	res, _, stderr := runSN(t, srv, "invite", "send", "doc1", "--email", "signer@x.com", "--from", "me@acme.com", "--no-email")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/document/doc1/invite")
	if got == nil {
		t.Fatalf("no invite request recorded")
	}
	if got.Query.Get("email") != "disable" {
		t.Errorf("--no-email must append ?email=disable, got %q", got.Query.Get("email"))
	}
	body := bodyMap(t, got.Body)
	if body["to"] != "signer@x.com" {
		t.Errorf("free-form invite must send to as a string email, got %v", body["to"])
	}
}

func TestInviteSend_BothSelectors_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "invite", "send", "doc1", "--to", "[]", "--email", "x@y.com")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 when both --to and --email are given", res.exitCode)
	}
}

func TestInviteSend_NeitherSelector_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "invite", "send", "doc1")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 when neither --to nor --email is given", res.exitCode)
	}
}

func TestInviteResend(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /fieldinvite/fi1/resend": {http.StatusOK, `{"status":"success"}`},
	})
	defer srv.Close()

	res, stdout, _ := runSN(t, srv, "invite", "resend", "fi1")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	if findReq(reqs, http.MethodPut, "/fieldinvite/fi1/resend") == nil {
		t.Fatalf("no resend request recorded")
	}
	if decodeStdout(t, stdout)["status"] != "resent" {
		t.Errorf("stdout = %q, want status resent", stdout)
	}
}

func TestInviteCancel(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /document/doc1/fieldinvitecancel": {http.StatusOK, `{"status":"success"}`},
	})
	defer srv.Close()

	res, stdout, _ := runSN(t, srv, "invite", "cancel", "doc1")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	if findReq(reqs, http.MethodPut, "/document/doc1/fieldinvitecancel") == nil {
		t.Fatalf("no cancel request recorded")
	}
	if decodeStdout(t, stdout)["status"] != "cancelled" {
		t.Errorf("stdout = %q, want status cancelled", stdout)
	}
}
