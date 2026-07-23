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
		"--customer-email", "a@b.com", "--subject", "Help", "--body", "hi")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/tickets" {
		t.Fatalf("request = %s %s, want POST /tickets", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	// The Gorgias create contract requires channel + via + from_agent at the
	// ticket level (default channel is api, so via derives to api).
	if body["channel"] != "api" || body["via"] != "api" || body["from_agent"] != false {
		t.Errorf("ticket body = %v, want channel=api/via=api/from_agent=false", body)
	}
	if body["subject"] != "Help" {
		t.Errorf("body.subject = %v, want Help", body["subject"])
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
	// The initial message also requires channel + via on every channel.
	if msg["channel"] != "api" || msg["via"] != "api" || msg["from_agent"] != false || msg["body_text"] != "hi" {
		t.Errorf("message = %v, want channel=api/via=api/from_agent=false/body_text=hi", msg)
	}
	sender, ok := msg["sender"].(map[string]any)
	if !ok || sender["email"] != "a@b.com" {
		t.Errorf("message.sender = %v, want {email: a@b.com}", msg["sender"])
	}
	if _, hasSource := msg["source"]; hasSource {
		t.Errorf("message.source = %v, want omitted for the api channel", msg["source"])
	}
}

func TestTicketCreateEmailSource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":10}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "create",
		"--customer-email", "a@b.com", "--subject", "Help", "--body", "hi",
		"--channel", "email", "--source-from", "support@acme.com",
		"--source-to", "a@b.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	body := decodeBody(t, got.Body)
	// Email channel derives via=email at both ticket and message level.
	if body["channel"] != "email" || body["via"] != "email" {
		t.Errorf("ticket body = %v, want channel=email/via=email", body)
	}
	msg := body["messages"].([]any)[0].(map[string]any)
	if msg["channel"] != "email" || msg["via"] != "email" {
		t.Errorf("message = %v, want channel=email/via=email", msg)
	}
	source, ok := msg["source"].(map[string]any)
	if !ok {
		t.Fatalf("message.source = %v, want a source object for the email channel", msg["source"])
	}
	from, ok := source["from"].(map[string]any)
	if !ok || from["address"] != "support@acme.com" {
		t.Errorf("source.from = %v, want {address: support@acme.com}", source["from"])
	}
	to, ok := source["to"].([]any)
	if !ok || len(to) != 1 || to[0].(map[string]any)["address"] != "a@b.com" {
		t.Errorf("source.to = %v, want [{address: a@b.com}]", source["to"])
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
		"--body", "thanks", "--from-agent", "--sender-email", "agent@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/tickets/42/messages" {
		t.Fatalf("request = %s %s, want POST /tickets/42/messages", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	// via is required on every message; the default channel is api, so via
	// derives to api and no source object is emitted.
	if body["channel"] != "api" || body["via"] != "api" || body["from_agent"] != true || body["body_text"] != "thanks" {
		t.Errorf("body = %v, want channel=api/via=api/from_agent/body_text", body)
	}
	sender, ok := body["sender"].(map[string]any)
	if !ok || sender["email"] != "agent@acme.com" {
		t.Errorf("body.sender = %v, want {email: agent@acme.com}", body["sender"])
	}
	if _, hasSource := body["source"]; hasSource {
		t.Errorf("body.source = %v, want omitted for the api channel", body["source"])
	}
}

func TestMessageCreateEmailSource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":101}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "message", "create", "42",
		"--body", "shipped", "--channel", "email", "--from-agent",
		"--sender-email", "support@acme.com",
		"--source-from", "support@acme.com",
		"--source-to", "jane@example.com", "--source-to", "cc@example.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	body := decodeBody(t, got.Body)
	// Email channel derives via=email and requires a source object.
	if body["channel"] != "email" || body["via"] != "email" {
		t.Errorf("body = %v, want channel=email/via=email", body)
	}
	source, ok := body["source"].(map[string]any)
	if !ok {
		t.Fatalf("body.source = %v, want a source object for the email channel", body["source"])
	}
	from, ok := source["from"].(map[string]any)
	if !ok || from["address"] != "support@acme.com" {
		t.Errorf("source.from = %v, want {address: support@acme.com}", source["from"])
	}
	to, ok := source["to"].([]any)
	if !ok || len(to) != 2 {
		t.Fatalf("source.to = %v, want two addresses", source["to"])
	}
	if to[0].(map[string]any)["address"] != "jane@example.com" ||
		to[1].(map[string]any)["address"] != "cc@example.com" {
		t.Errorf("source.to = %v, want [jane, cc] addresses", to)
	}
}

func TestMessageCreateViaOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":102}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "create", "42",
		"--body", "note", "--channel", "internal-note", "--via", "api",
		"--from-agent", "--sender-email", "agent@acme.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	// An explicit --via wins over the channel-derived default.
	if body["channel"] != "internal-note" || body["via"] != "api" {
		t.Errorf("body = %v, want channel=internal-note/via=api (explicit override)", body)
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
