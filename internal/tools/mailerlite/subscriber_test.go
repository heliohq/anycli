package mailerlite

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestSubscriberList_StatusFilterAndPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[],"meta":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "subscriber", "list", "--status", "active", "--limit", "50", "--cursor", "abc", "--include", "groups")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/subscribers" {
		t.Errorf("request = %s %s, want GET /api/subscribers", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("filter[status]") != "active" {
		t.Errorf("filter[status] = %q", q.Get("filter[status]"))
	}
	if q.Get("limit") != "50" || q.Get("cursor") != "abc" || q.Get("include") != "groups" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestSubscriberList_NoLimitFlag_OmitsLimit(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "subscriber", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, ok := parseQuery(t, got.Query)["limit"]; ok {
		t.Errorf("limit should be absent when --limit not passed, query = %q", got.Query)
	}
}

func TestSubscriberGet_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "subscriber", "get", "user@example.com"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/subscribers/user@example.com" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestSubscriberCreate_BuildsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "subscriber", "create",
		"--email", "new@example.com",
		"--fields", `{"name":"Jo"}`,
		"--groups", "g1, g2",
		"--status", "active")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/subscribers" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q", got.ContentType)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "new@example.com" || body["status"] != "active" {
		t.Errorf("body = %v", body)
	}
	if fields, ok := body["fields"].(map[string]any); !ok || fields["name"] != "Jo" {
		t.Errorf("fields = %v", body["fields"])
	}
	groups, _ := body["groups"].([]any)
	if !reflect.DeepEqual(groups, []any{"g1", "g2"}) {
		t.Errorf("groups = %v, want [g1 g2]", body["groups"])
	}
	// resubscribe was not passed → must be omitted.
	if _, ok := body["resubscribe"]; ok {
		t.Errorf("resubscribe should be omitted when unset, body = %v", body)
	}
}

func TestSubscriberUpdate_OnlyChangedFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "subscriber", "update", "42", "--status", "unsubscribed"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/api/subscribers/42" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["status"] != "unsubscribed" {
		t.Errorf("status = %v", body["status"])
	}
	if _, ok := body["email"]; ok {
		t.Errorf("email must not be sent on update, body = %v", body)
	}
}

func TestSubscriberDelete_204ReceiptEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "subscriber", "delete", "7")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/api/subscribers/7" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if want := `{"success":true}`; strings.TrimSpace(stdout) != want {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
}

func TestSubscriberCount_LimitZero(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"total":100}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "subscriber", "count"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/subscribers" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("limit") != "0" {
		t.Errorf("limit = %q, want 0", q.Get("limit"))
	}
}

func TestSubscriberActivity_PageParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "subscriber", "activity", "9", "--log-name", "email_open", "--limit", "10", "--page", "2")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/subscribers/9/activity-log" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("filter[log_name]") != "email_open" || q.Get("limit") != "10" || q.Get("page") != "2" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestSubscriberForget(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "subscriber", "forget", "5"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/subscribers/5/forget" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}
