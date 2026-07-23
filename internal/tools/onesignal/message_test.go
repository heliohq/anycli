package onesignal

import (
	"net/http"
	"testing"
)

func TestMessageSend_PushSegment_KeySchemeAndAppIDInBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"n-1","recipients":5}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "message", "send",
		"--channel", "push", "--segment", "Subscribed Users",
		"--heading", "Hi", "--content", "Hello world")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/notifications" {
		t.Errorf("request = %s %s, want POST /notifications", got.Method, got.Path)
	}
	if got.Auth != "Key "+testKey {
		t.Errorf("Authorization = %q, want %q", got.Auth, "Key "+testKey)
	}
	body := decodeBody(t, got.Body)
	if body["app_id"] != testAppID {
		t.Errorf("app_id in body = %v, want %s", body["app_id"], testAppID)
	}
	if body["target_channel"] != "push" {
		t.Errorf("target_channel = %v", body["target_channel"])
	}
	segs, ok := body["included_segments"].([]any)
	if !ok || len(segs) != 1 || segs[0] != "Subscribed Users" {
		t.Errorf("included_segments = %v", body["included_segments"])
	}
	contents, ok := body["contents"].(map[string]any)
	if !ok || contents["en"] != "Hello world" {
		t.Errorf("contents = %v", body["contents"])
	}
	headings, ok := body["headings"].(map[string]any)
	if !ok || headings["en"] != "Hi" {
		t.Errorf("headings = %v", body["headings"])
	}
	if stdout != `{"id":"n-1","recipients":5}`+"\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestMessageSend_Email_UsesEmailSubjectAndBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"n-2"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send",
		"--channel", "email", "--email", "a@b.com",
		"--heading", "Subject line", "--content", "<p>Body</p>")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if body["email_subject"] != "Subject line" {
		t.Errorf("email_subject = %v", body["email_subject"])
	}
	if body["email_body"] != "<p>Body</p>" {
		t.Errorf("email_body = %v", body["email_body"])
	}
	to, ok := body["email_to"].([]any)
	if !ok || len(to) != 1 || to[0] != "a@b.com" {
		t.Errorf("email_to = %v", body["email_to"])
	}
	if _, present := body["include_email_tokens"]; present {
		t.Errorf("email send should use email_to, not the deprecated include_email_tokens: %v", body)
	}
	if _, present := body["contents"]; present {
		t.Errorf("email send should not set contents, body = %v", body)
	}
}

func TestMessageSend_FiltersTargeting_PassesThroughJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"n-3"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send",
		"--filters", `[{"field":"tag","key":"vip","relation":"=","value":"true"}]`,
		"--content", "Hey")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	filters, ok := body["filters"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("filters = %v", body["filters"])
	}
}

func TestMessageSend_NoTargeting_UsageExit2NoHTTP(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "message", "send", "--content", "hi")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, but server saw %s %s", got.Method, got.Path)
	}
	if stderr == "" {
		t.Error("expected a usage error on stderr")
	}
}

func TestMessageSend_TwoTargeting_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send",
		"--segment", "A", "--subscription-id", "sub-1", "--content", "hi")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, server saw %s %s", got.Method, got.Path)
	}
}

func TestMessageSend_MissingContent_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send", "--segment", "A")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, server saw %s %s", got.Method, got.Path)
	}
}

func TestMessageSend_BadChannel_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send", "--channel", "carrier-pigeon", "--segment", "A", "--content", "hi")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestMessageList_AppIDAndPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"notifications":[],"total_count":0}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "list", "--limit", "10", "--offset", "20")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/notifications" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("app_id") != testAppID || q.Get("limit") != "10" || q.Get("offset") != "20" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestMessageList_OmitsUnsetPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	q := parseQuery(t, got.Query)
	if _, ok := q["limit"]; ok {
		t.Errorf("limit should be absent, query = %q", got.Query)
	}
	if _, ok := q["offset"]; ok {
		t.Errorf("offset should be absent, query = %q", got.Query)
	}
}

func TestMessageGet_PathAndAppIDQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"n-9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "get", "--id", "n-9")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/notifications/n-9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("app_id") != testAppID {
		t.Errorf("app_id = %q", q.Get("app_id"))
	}
}

func TestMessageGet_MissingID_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "get")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected")
	}
}

func TestMessageCancel_DeleteWithAppID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "cancel", "--id", "n-7")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/notifications/n-7" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("app_id") != testAppID {
		t.Errorf("app_id = %q", q.Get("app_id"))
	}
}
