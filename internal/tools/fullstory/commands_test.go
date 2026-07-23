package fullstory

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

func TestSessionList_Path_Query_Passthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":"d:s","fs_url":"https://app.fullstory.com/x"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "session", "list", "--email", "a@b.com", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/sessions" {
		t.Errorf("request = %s %s, want GET /v2/sessions", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("email") != "a@b.com" || q.Get("limit") != "5" {
		t.Errorf("query = %q, want email + limit", got.Query)
	}
	if q.Has("uid") {
		t.Errorf("query = %q, should omit empty uid", got.Query)
	}
	if !strings.Contains(stdout, `"results"`) {
		t.Errorf("stdout = %q, want results passthrough", stdout)
	}
}

func TestSessionList_UID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "session", "list", "--uid", "user-42"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if q := parseQuery(t, got.Query); q.Get("uid") != "user-42" {
		t.Errorf("query = %q, want uid=user-42", got.Query)
	}
}

func TestUserGet_PathAndSchema(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"FS1","uid":"u1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "get", "--id", "FS_USER_ID_12345", "--include-schema")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/users/FS_USER_ID_12345" {
		t.Errorf("request = %s %s, want GET /v2/users/FS_USER_ID_12345", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("include_schema") != "true" {
		t.Errorf("query = %q, want include_schema=true", got.Query)
	}
	if !strings.Contains(stdout, `"id":"FS1"`) {
		t.Errorf("stdout = %q, want object passthrough", stdout)
	}
}

func TestUserGet_RequiresID(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, &capturedRequest{})
	defer srv.Close()
	if code, _, stderr := run(t, srv, "user", "get"); code != 2 || !strings.Contains(stderr, "requires --id") {
		t.Errorf("code=%d stderr=%q, want exit 2 with --id usage", code, stderr)
	}
}

func TestUserList_Query(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "user", "list", "--uid", "u9"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v2/users" {
		t.Errorf("path = %q, want /v2/users", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("uid") != "u9" {
		t.Errorf("query = %q, want uid=u9", got.Query)
	}
}

func TestUserUpsert_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"FS1","uid":"u1","app_url":"https://x"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "upsert",
		"--uid", "u1", "--display-name", "Ada", "--email", "ada@x.com",
		"--prop", "plan=paid", "--prop", "total_spent=14.55", "--prop", "vip=true")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/users" {
		t.Errorf("request = %s %s, want POST /v2/users", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["uid"] != "u1" || body["display_name"] != "Ada" || body["email"] != "ada@x.com" {
		t.Errorf("body = %v, want uid/display_name/email set", body)
	}
	props, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("body.properties = %v, want object", body["properties"])
	}
	if props["plan"] != "paid" {
		t.Errorf("props.plan = %v, want string paid", props["plan"])
	}
	if props["total_spent"] != 14.55 {
		t.Errorf("props.total_spent = %v (%T), want float 14.55", props["total_spent"], props["total_spent"])
	}
	if props["vip"] != true {
		t.Errorf("props.vip = %v (%T), want bool true", props["vip"], props["vip"])
	}
}

func TestUserUpsert_RequiresUID(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, &capturedRequest{})
	defer srv.Close()
	if code, _, stderr := run(t, srv, "user", "upsert"); code != 2 || !strings.Contains(stderr, "requires --uid") {
		t.Errorf("code=%d stderr=%q, want exit 2 with --uid usage", code, stderr)
	}
}

func TestEventCreate_ByUser(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"created":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event", "create", "--name", "Support Ticket", "--uid", "u1", "--prop", "priority=Normal")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/events" {
		t.Errorf("request = %s %s, want POST /v2/events", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Support Ticket" {
		t.Errorf("body.name = %v, want Support Ticket", body["name"])
	}
	user, ok := body["user"].(map[string]any)
	if !ok || user["uid"] != "u1" {
		t.Errorf("body.user = %v, want {uid:u1}", body["user"])
	}
	if _, hasSession := body["session"]; hasSession {
		t.Errorf("body = %v, should not carry session when identifying by uid alone", body)
	}
}

func TestEventCreate_BySession(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "event", "create", "--name", "e", "--session-id", "dev:sess"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	session, ok := body["session"].(map[string]any)
	if !ok || session["id"] != "dev:sess" {
		t.Errorf("body.session = %v, want {id:dev:sess}", body["session"])
	}
	if _, hasUser := body["user"]; hasUser {
		t.Errorf("body = %v, should not carry user when identifying by session", body)
	}
}

func TestEventCreate_UseRecent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "event", "create", "--name", "e", "--uid", "u1", "--use-recent"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	user, _ := body["user"].(map[string]any)
	session, _ := body["session"].(map[string]any)
	if user["uid"] != "u1" || session["use_most_recent"] != true {
		t.Errorf("body = %v, want user.uid + session.use_most_recent", body)
	}
}

func TestEventCreate_UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no name", []string{"event", "create", "--uid", "u"}, "requires --name"},
		{"no id", []string{"event", "create", "--name", "e"}, "requires --uid or --session-id"},
		{"both ids", []string{"event", "create", "--name", "e", "--uid", "u", "--session-id", "s"}, "only one of --uid or --session-id"},
		{"recent without uid", []string{"event", "create", "--name", "e", "--session-id", "s", "--use-recent"}, "requires --uid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{}`, &got)
			defer srv.Close()
			code, _, stderr := run(t, srv, tc.args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if got.Method != "" {
				t.Errorf("issued HTTP %s, want no request", got.Method)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want %q", stderr, tc.want)
			}
		})
	}
}

func TestBadPropIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	code, _, stderr := run(t, srv, "user", "upsert", "--uid", "u", "--prop", "novalue")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("issued HTTP %s, want no request for a bad --prop", got.Method)
	}
	if !strings.Contains(stderr, "key=value") {
		t.Errorf("stderr = %q, want key=value guidance", stderr)
	}
}

func TestMe_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"role":"ADMIN"}`, &got)
	defer srv.Close()
	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/me" {
		t.Errorf("path = %q, want /me", got.Path)
	}
	if !strings.Contains(stdout, `"role":"ADMIN"`) {
		t.Errorf("stdout = %q, want role passthrough", stdout)
	}
}
