package freshservice

import (
	"net/url"
	"testing"
)

func TestRequesterListWithEmail(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/requesters": {body: `{"requesters":[{"id":1}]}`},
	}, captured)
	defer srv.Close()

	code, out, errStr := run(t, srv, "requester", "list", "--email", "user@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	q, _ := url.ParseQuery(captured["/requesters"].Query)
	if q.Get("email") != "user@acme.com" {
		t.Fatalf("email not sent: %q", captured["/requesters"].Query)
	}
	m := decodeJSON(t, out)
	if _, ok := m["items"]; !ok {
		t.Fatalf("requester list should project items: %v", m)
	}
}

func TestRequesterGet(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/requesters/3": {body: `{"requester":{"id":3,"first_name":"Ada"}}`},
	}, captured)
	defer srv.Close()

	code, out, _ := run(t, srv, "requester", "get", "3")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	m := decodeJSON(t, out)
	if m["first_name"] != "Ada" {
		t.Fatalf("requester get not unwrapped: %v", m)
	}
}

func TestAgentListWithEmail(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/agents": {body: `{"agents":[{"id":8}]}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "agent", "list", "--email", "agent@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	q, _ := url.ParseQuery(captured["/agents"].Query)
	if q.Get("email") != "agent@acme.com" {
		t.Fatalf("email not sent: %q", captured["/agents"].Query)
	}
}

func TestAgentGet(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/agents/8": {body: `{"agent":{"id":8}}`},
	}, captured)
	defer srv.Close()

	code, out, _ := run(t, srv, "agent", "get", "8")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	m := decodeJSON(t, out)
	if m["id"].(float64) != 8 {
		t.Fatalf("agent get not unwrapped: %v", m)
	}
}

func TestGroupList(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/groups": {body: `{"groups":[{"id":2,"name":"IT"}]}`},
	}, captured)
	defer srv.Close()

	code, out, _ := run(t, srv, "group", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	m := decodeJSON(t, out)
	if items, ok := m["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("group list items wrong: %v", m)
	}
}

func TestAssetListWithFilter(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/assets": {body: `{"assets":[{"display_id":42}]}`},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "asset", "list", "--filter", "name:'MacBook'")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	q, _ := url.ParseQuery(captured["/assets"].Query)
	if got := q.Get("filter"); got != `"name:'MacBook'"` {
		t.Fatalf("filter = %q, want quoted expression", got)
	}
}

func TestAssetGetByDisplayID(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/assets/42": {body: `{"asset":{"display_id":42,"name":"Ada's MacBook"}}`},
	}, captured)
	defer srv.Close()

	code, out, _ := run(t, srv, "asset", "get", "42")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	m := decodeJSON(t, out)
	if m["name"] != "Ada's MacBook" {
		t.Fatalf("asset get not unwrapped: %v", m)
	}
}
