package twitch

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "get"}, map[string]string{EnvClientID: testClientID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvToken+" is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingClientID(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "get"}, map[string]string{EnvToken: testToken})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvClientID+" is not set") {
		t.Errorf("stderr = %q, want the missing-client-id message", errBuf.String())
	}
}

func TestUserGet_SelfSendsBothHeaders(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"42","login":"alice","display_name":"Alice"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "get")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/users" {
		t.Errorf("request = %s %s, want GET /users", got.Method, got.Path)
	}
	if got.Auth != "Bearer "+testToken {
		t.Errorf("Authorization = %q, want Bearer %s", got.Auth, testToken)
	}
	if got.ClientID != testClientID {
		t.Errorf("Client-Id = %q, want %q", got.ClientID, testClientID)
	}
	// Single lookup unwraps data[0].
	obj := decodeOut(t, stdout)
	if obj["login"] != "alice" || obj["id"] != "42" {
		t.Errorf("stdout = %q, want the unwrapped user object", stdout)
	}
}

func TestUserGet_MultipleFiltersEmitList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"1"},{"id":"2"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "get", "--login", "a", "--login", "b")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if logins := q["login"]; len(logins) != 2 || logins[0] != "a" || logins[1] != "b" {
		t.Errorf("login params = %v, want [a b]", logins)
	}
	out := decodeOut(t, stdout)
	if _, ok := out["data"]; !ok {
		t.Errorf("stdout = %q, want a list envelope with data", stdout)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"error":"Unauthorized","status":401,"message":"invalid access token"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "user", "get")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "invalid access token") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":"Bad Request","status":400,"message":"missing broadcaster_id"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "channel", "get", "--broadcaster-id", "42")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	env := decodeOut(t, strings.TrimSpace(stderr))
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("stderr = %q, want a JSON error envelope", stderr)
	}
	if errObj["kind"] != "api" {
		t.Errorf("error.kind = %v, want api", errObj["kind"])
	}
	if errObj["status"] != float64(400) {
		t.Errorf("error.status = %v, want 400", errObj["status"])
	}
}

func TestUsageError_MissingRequiredFlag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search", "channels")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", code)
	}
	if !strings.Contains(stderr, "requires --query") {
		t.Errorf("stderr = %q, want the missing-query usage message", stderr)
	}
	if got.Path != "" {
		t.Errorf("no HTTP call should be made on a usage error; hit %s", got.Path)
	}
}

func TestUsageError_UnknownSubcommand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an unknown subcommand", code)
	}
}

func TestStreamList_CursorEcho(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"s1"}],"pagination":{"cursor":"NEXT"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "stream", "list", "--first", "5", "--after", "PREV", "--game-id", "99")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("first") != "5" || q.Get("after") != "PREV" || q.Get("game_id") != "99" {
		t.Errorf("query = %q, want first=5 after=PREV game_id=99", got.Query)
	}
	out := decodeOut(t, stdout)
	if out["cursor"] != "NEXT" {
		t.Errorf("cursor = %v, want NEXT echoed to the caller", out["cursor"])
	}
}

func TestChannelUpdate_ResolvesSelfBroadcasterID(t *testing.T) {
	captured := map[string]capturedRequest{}
	var order []string
	routes := map[string]routeHandler{
		"/users":    {status: http.StatusOK, response: `{"data":[{"id":"777","login":"self"}]}`},
		"/channels": {status: http.StatusNoContent, response: ""},
	}
	srv := newMultiServer(t, routes, captured, &order)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "channel", "update", "--title", "New Title")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// Self id resolved via Get Users first, then PATCH /channels with it.
	if len(order) != 2 || order[0] != "/users" || order[1] != "/channels" {
		t.Fatalf("request order = %v, want [/users /channels]", order)
	}
	patch := captured["/channels"]
	if patch.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", patch.Method)
	}
	if q := parseQuery(t, patch.Query); q.Get("broadcaster_id") != "777" {
		t.Errorf("broadcaster_id = %q, want the resolved self id 777", q.Get("broadcaster_id"))
	}
	if body := decodeBody(t, patch.Body); body["title"] != "New Title" {
		t.Errorf("PATCH body = %v, want title=New Title", body)
	}
	if out := decodeOut(t, stdout); out["updated"] != true {
		t.Errorf("stdout = %q, want an updated receipt", stdout)
	}
}

func TestChannelUpdate_NoFieldsIsUsageError(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{}, captured, nil)
	defer srv.Close()

	code, _, stderr := run(t, srv, "channel", "update")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "at least one of") {
		t.Errorf("stderr = %q, want the no-fields usage message", stderr)
	}
	if len(captured) != 0 {
		t.Errorf("no HTTP call should be made; captured %v", captured)
	}
}

func TestChatSend_SenderIsSelfBroadcasterDefaultsSelf(t *testing.T) {
	captured := map[string]capturedRequest{}
	var order []string
	routes := map[string]routeHandler{
		"/users":         {status: http.StatusOK, response: `{"data":[{"id":"555"}]}`},
		"/chat/messages": {status: http.StatusOK, response: `{"data":[{"message_id":"m1","is_sent":true}]}`},
	}
	srv := newMultiServer(t, routes, captured, &order)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "chat", "send", "--message", "hello chat")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, captured["/chat/messages"].Body)
	if body["sender_id"] != "555" {
		t.Errorf("sender_id = %v, want self id 555", body["sender_id"])
	}
	if body["broadcaster_id"] != "555" {
		t.Errorf("broadcaster_id = %v, want self id 555 by default", body["broadcaster_id"])
	}
	if body["message"] != "hello chat" {
		t.Errorf("message = %v, want hello chat", body["message"])
	}
	if out := decodeOut(t, stdout); out["message_id"] != "m1" {
		t.Errorf("stdout = %q, want the unwrapped send result", stdout)
	}
}

func TestChatSend_MissingMessageUsageError(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{}, captured, nil)
	defer srv.Close()

	code, _, stderr := run(t, srv, "chat", "send")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "requires --message") {
		t.Errorf("stderr = %q, want the missing-message usage message", stderr)
	}
}

func TestChatters_UsesModeratorSelf(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/users":         {status: http.StatusOK, response: `{"data":[{"id":"900"}]}`},
		"/chat/chatters": {status: http.StatusOK, response: `{"data":[{"user_login":"bob"}],"pagination":{"cursor":""}}`},
	}
	srv := newMultiServer(t, routes, captured, nil)
	defer srv.Close()

	code, _, _ := run(t, srv, "chat", "chatters", "--broadcaster-id", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, captured["/chat/chatters"].Query)
	if q.Get("moderator_id") != "900" {
		t.Errorf("moderator_id = %q, want self id 900", q.Get("moderator_id"))
	}
	if q.Get("broadcaster_id") != "123" {
		t.Errorf("broadcaster_id = %q, want the explicit 123", q.Get("broadcaster_id"))
	}
}

func TestClipList_RequiresExactlyOneSelector(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{}, captured, nil)
	defer srv.Close()

	code, _, stderr := run(t, srv, "clip", "list")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "exactly one of") {
		t.Errorf("stderr = %q, want the selector usage message", stderr)
	}
}

func TestSubscriberList_AlwaysSelfBroadcaster(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/users":         {status: http.StatusOK, response: `{"data":[{"id":"333"}]}`},
		"/subscriptions": {status: http.StatusOK, response: `{"data":[],"pagination":{"cursor":""}}`},
	}
	srv := newMultiServer(t, routes, captured, nil)
	defer srv.Close()

	code, _, _ := run(t, srv, "subscriber", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, captured["/subscriptions"].Query)
	if q.Get("broadcaster_id") != "333" {
		t.Errorf("broadcaster_id = %q, want self id 333", q.Get("broadcaster_id"))
	}
}
