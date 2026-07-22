package gorgias

import (
	"net/url"
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

func TestTicketList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[],"meta":{"next_cursor":null}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "list",
		"--view", "7", "--customer", "12", "--limit", "50",
		"--order-by", "updated_datetime:desc", "--cursor", "abc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/tickets" {
		t.Fatalf("request = %s %s, want GET /tickets", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("view_id") != "7" || q.Get("customer_id") != "12" || q.Get("limit") != "50" ||
		q.Get("order_by") != "updated_datetime:desc" || q.Get("cursor") != "abc" {
		t.Errorf("query = %s, missing expected ticket filters", got.Query)
	}
}

func TestTicketGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":5}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "get", "5")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/tickets/5" {
		t.Errorf("request = %s %s, want GET /tickets/5", got.Method, got.Path)
	}
}

func TestTicketCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":9}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "create",
		"--customer-email", "a@b.com", "--subject", "Help", "--body", "hi", "--channel", "email")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/tickets" {
		t.Fatalf("request = %s %s, want POST /tickets", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["channel"] != "email" || body["subject"] != "Help" {
		t.Errorf("body = %v, want channel/subject set", body)
	}
	cust, ok := body["customer"].(map[string]any)
	if !ok || cust["email"] != "a@b.com" {
		t.Errorf("body.customer = %v, want {email: a@b.com}", body["customer"])
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("body.messages = %v, want one message", body["messages"])
	}
	msg := msgs[0].(map[string]any)
	if msg["channel"] != "email" || msg["from_agent"] != false || msg["body_text"] != "hi" {
		t.Errorf("message = %v, want channel/from_agent/body_text", msg)
	}
	sender, ok := msg["sender"].(map[string]any)
	if !ok || sender["email"] != "a@b.com" {
		t.Errorf("message.sender = %v, want {email: a@b.com}", msg["sender"])
	}
}

func TestTicketUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{"id":3}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "update", "3",
		"--status", "closed", "--assignee", "88", "--priority", "high", "--tag", "vip", "--tag", "urgent")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "PUT" || got.Path != "/tickets/3" {
		t.Fatalf("request = %s %s, want PUT /tickets/3", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["status"] != "closed" || body["priority"] != "high" {
		t.Errorf("body = %v, want status/priority", body)
	}
	assignee, ok := body["assignee_user"].(map[string]any)
	if !ok || assignee["id"] != float64(88) {
		t.Errorf("body.assignee_user = %v, want {id: 88}", body["assignee_user"])
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("body.tags = %v, want two tag objects", body["tags"])
	}
	if tags[0].(map[string]any)["name"] != "vip" || tags[1].(map[string]any)["name"] != "urgent" {
		t.Errorf("tags = %v, want [{name:vip},{name:urgent}]", tags)
	}
}

func TestMessageList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "list", "42", "--limit", "10", "--cursor", "c1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/tickets/42/messages" {
		t.Fatalf("request = %s %s, want GET /tickets/42/messages", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "10" || q.Get("cursor") != "c1" {
		t.Errorf("query = %s, want limit/cursor", got.Query)
	}
}

func TestMessageCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":100}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "message", "create", "42",
		"--body", "thanks", "--channel", "email", "--from-agent", "--sender-email", "agent@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/tickets/42/messages" {
		t.Fatalf("request = %s %s, want POST /tickets/42/messages", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["channel"] != "email" || body["from_agent"] != true || body["body_text"] != "thanks" {
		t.Errorf("body = %v, want channel/from_agent/body_text", body)
	}
	sender, ok := body["sender"].(map[string]any)
	if !ok || sender["email"] != "agent@acme.com" {
		t.Errorf("body.sender = %v, want {email: agent@acme.com}", body["sender"])
	}
}

func TestCustomerList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "customer", "list", "--email", "x@y.com", "--name", "Xavier", "--external-id", "cus_1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/customers" {
		t.Fatalf("request = %s %s, want GET /customers", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("email") != "x@y.com" || q.Get("name") != "Xavier" || q.Get("external_id") != "cus_1" {
		t.Errorf("query = %s, want email/name/external_id", got.Query)
	}
}

func TestCustomerGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":11}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "customer", "get", "11")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/customers/11" {
		t.Errorf("request = %s %s, want GET /customers/11", got.Method, got.Path)
	}
}

func TestUserListAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "user", "list", "--limit", "5"); code != 0 {
		t.Fatalf("user list exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/users" {
		t.Errorf("request = %s %s, want GET /users", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("limit") != "5" {
		t.Errorf("query = %s, want limit=5", got.Query)
	}

	if code, _, _ := run(t, srv, "user", "get", "77"); code != 0 {
		t.Fatalf("user get exit = %d, want 0", code)
	}
	if got.Path != "/users/77" {
		t.Errorf("path = %s, want /users/77", got.Path)
	}
}

func TestTagList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "tag", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/tags" {
		t.Errorf("request = %s %s, want GET /tags", got.Method, got.Path)
	}
}

func TestViewListAndItems(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "view", "list", "--category", "user"); code != 0 {
		t.Fatalf("view list exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/views" {
		t.Errorf("request = %s %s, want GET /views", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("category") != "user" {
		t.Errorf("query = %s, want category=user", got.Query)
	}

	if code, _, _ := run(t, srv, "view", "items", "9", "--cursor", "z"); code != 0 {
		t.Fatalf("view items exit = %d, want 0", code)
	}
	if got.Path != "/views/9/items" {
		t.Errorf("path = %s, want /views/9/items", got.Path)
	}
	if parseQuery(t, got.Query).Get("cursor") != "z" {
		t.Errorf("query = %s, want cursor=z", got.Query)
	}
}

func TestSatisfactionList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "satisfaction", "list", "--limit", "20")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != "GET" || got.Path != "/satisfaction-surveys" {
		t.Errorf("request = %s %s, want GET /satisfaction-surveys", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("limit") != "20" {
		t.Errorf("query = %s, want limit=20", got.Query)
	}
}
