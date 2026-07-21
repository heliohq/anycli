package instantly

import (
	"net/http"
	"testing"
)

func TestEmailListMapsFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	run(t, srv, "email", "list", "--campaign-id", "c1", "--eaccount", "s@x.com", "--is-unread", "true", "--limit", "10")
	if got.Method != http.MethodGet || got.Path != "/emails" {
		t.Fatalf("got %s %s, want GET /emails", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("campaign_id") != "c1" || q.Get("eaccount") != "s@x.com" || q.Get("is_unread") != "true" || q.Get("limit") != "10" {
		t.Fatalf("query = %v", q)
	}
}

func TestEmailReplyBuildsRequiredBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "email", "reply",
		"--eaccount", "s@x.com", "--reply-to-uuid", "uuid1", "--subject", "Re: hi", "--body", "hello", "--cc", "c@x.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/emails/reply" {
		t.Fatalf("got %s %s, want POST /emails/reply", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["eaccount"] != "s@x.com" || body["reply_to_uuid"] != "uuid1" || body["subject"] != "Re: hi" || body["body"] != "hello" {
		t.Fatalf("body = %v", body)
	}
	if body["cc_address_email_list"] != "c@x.com" {
		t.Fatalf("cc = %v", body["cc_address_email_list"])
	}
}

func TestEmailReplyMissingRequiredIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "email", "reply", "--eaccount", "s@x.com", "--reply-to-uuid", "uuid1")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for missing --subject/--body", exit)
	}
}

func TestEmailUnreadCountAndMarkRead(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"count":3}`, &got)
	defer srv.Close()
	run(t, srv, "email", "unread-count")
	if got.Method != http.MethodGet || got.Path != "/emails/unread/count" {
		t.Fatalf("unread-count got %s %s", got.Method, got.Path)
	}

	var got2 capturedRequest
	srv2 := newServer(t, http.StatusOK, `{"ok":true}`, &got2)
	defer srv2.Close()
	run(t, srv2, "email", "mark-read", "--thread-id", "t1")
	if got2.Method != http.MethodPost || got2.Path != "/emails/threads/t1/mark-as-read" {
		t.Fatalf("mark-read got %s %s", got2.Method, got2.Path)
	}
}

func TestAccountListAndActions(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()
	run(t, srv, "account", "list", "--status", "1", "--provider-code", "2")
	q := parseQuery(t, got.Query)
	if got.Path != "/accounts" || q.Get("status") != "1" || q.Get("provider_code") != "2" {
		t.Fatalf("account list path=%s query=%v", got.Path, q)
	}

	for _, tc := range []struct {
		action string
		path   string
	}{
		{"pause", "/accounts/s@x.com/pause"},
		{"resume", "/accounts/s@x.com/resume"},
	} {
		var g capturedRequest
		s := newServer(t, http.StatusOK, `{}`, &g)
		run(t, s, "account", tc.action, "--email", "s@x.com")
		s.Close()
		if g.Method != http.MethodPost || g.Path != tc.path {
			t.Fatalf("%s got %s %s, want POST %s", tc.action, g.Method, g.Path, tc.path)
		}
	}
}

func TestAccountWarmupAnalyticsSplitsEmails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "account", "warmup-analytics", "--emails", "a@x.com, b@x.com ,c@x.com")
	if got.Method != http.MethodPost || got.Path != "/accounts/warmup-analytics" {
		t.Fatalf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	emails, ok := body["emails"].([]any)
	if !ok || len(emails) != 3 || emails[0] != "a@x.com" || emails[1] != "b@x.com" || emails[2] != "c@x.com" {
		t.Fatalf("emails = %v", body["emails"])
	}
}

func TestLeadListGroupCommands(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"lead-list", "list", "--search", "q"}, http.MethodGet, "/lead-lists"},
		{[]string{"lead-list", "get", "--id", "ll1"}, http.MethodGet, "/lead-lists/ll1"},
		{[]string{"lead-list", "create", "--name", "Prospects"}, http.MethodPost, "/lead-lists"},
		{[]string{"lead-list", "update", "--id", "ll1", "--data", `{"name":"x"}`}, http.MethodPatch, "/lead-lists/ll1"},
		{[]string{"lead-list", "delete", "--id", "ll1"}, http.MethodDelete, "/lead-lists/ll1"},
		{[]string{"lead-list", "verification-stats", "--id", "ll1"}, http.MethodGet, "/lead-lists/ll1/verification-stats"},
	} {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{}`, &got)
		exit, _, _ := run(t, srv, tc.args...)
		srv.Close()
		if exit != 0 {
			t.Fatalf("%v exit = %d", tc.args, exit)
		}
		if got.Method != tc.method || got.Path != tc.path {
			t.Fatalf("%v got %s %s, want %s %s", tc.args, got.Method, got.Path, tc.method, tc.path)
		}
	}
}

func TestVerifyCreateAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"status":"pending"}`, &got)
	defer srv.Close()
	run(t, srv, "verify", "create", "--email", "a@x.com")
	if got.Method != http.MethodPost || got.Path != "/email-verification" {
		t.Fatalf("verify create got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@x.com" {
		t.Fatalf("email = %v", body["email"])
	}

	var got2 capturedRequest
	srv2 := newServer(t, http.StatusOK, `{"status":"verified"}`, &got2)
	defer srv2.Close()
	run(t, srv2, "verify", "get", "--email", "a@x.com")
	if got2.Method != http.MethodGet || got2.Path != "/email-verification/a@x.com" {
		t.Fatalf("verify get got %s %s", got2.Method, got2.Path)
	}
}

func TestJobListAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()
	run(t, srv, "job", "list", "--status", "running")
	if got.Path != "/background-jobs" {
		t.Fatalf("job list path = %s", got.Path)
	}

	var got2 capturedRequest
	srv2 := newServer(t, http.StatusOK, `{"id":"j1","status":"success"}`, &got2)
	defer srv2.Close()
	run(t, srv2, "job", "get", "--id", "j1")
	if got2.Method != http.MethodGet || got2.Path != "/background-jobs/j1" {
		t.Fatalf("job get got %s %s", got2.Method, got2.Path)
	}
}
