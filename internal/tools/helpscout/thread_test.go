package helpscout

import (
	"net/http"
	"strings"
	"testing"
)

func TestThreadList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"_embedded":{"threads":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "thread", "list", "7", "--page", "3")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/conversations/7/threads" {
		t.Errorf("path = %s", got.Path)
	}
	if parseQuery(t, got.Query).Get("page") != "3" {
		t.Errorf("page not set: %s", got.Query)
	}
}

func TestThreadReply_DefaultsCustomerFromPrimary(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/conversations/7":       {status: http.StatusOK, response: `{"id":7,"primaryCustomer":{"id":314}}`},
		"/conversations/7/reply": {status: http.StatusCreated, response: ``, header: map[string]string{"Resource-Id": "900"}},
	}, captured)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "thread", "reply", "7", "--text", "thanks!", "--status", "closed")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	// The conversation GET must have happened to resolve the customer.
	if _, ok := captured["/conversations/7"]; !ok {
		t.Fatal("expected a GET /conversations/7 to resolve primaryCustomer")
	}
	reply := captured["/conversations/7/reply"]
	if reply.Method != http.MethodPost {
		t.Fatalf("method = %s", reply.Method)
	}
	body := decodeBody(t, reply.Body)
	cust, _ := body["customer"].(map[string]any)
	if cust["id"] != float64(314) {
		t.Errorf("customer.id = %v, want defaulted 314", body["customer"])
	}
	if body["text"] != "thanks!" || body["status"] != "closed" {
		t.Errorf("body = %v", body)
	}
	rec := decodeBody(t, []byte(stdout))
	if rec["id"] != "900" || rec["status"] != "created" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

func TestThreadReply_ExplicitCustomerSkipsGet(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/conversations/7/reply": {status: http.StatusCreated, response: ``, header: map[string]string{"Resource-Id": "901"}},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "thread", "reply", "7", "--text", "hi", "--customer-id", "42", "--draft")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, ok := captured["/conversations/7"]; ok {
		t.Error("did not expect a GET when --customer-id is explicit")
	}
	body := decodeBody(t, captured["/conversations/7/reply"].Body)
	cust, _ := body["customer"].(map[string]any)
	if cust["id"] != float64(42) {
		t.Errorf("customer.id = %v", body["customer"])
	}
	if body["draft"] != true {
		t.Errorf("draft = %v", body["draft"])
	}
}

func TestThreadNote(t *testing.T) {
	var got capturedRequest
	srv := newHeaderServer(t, http.StatusCreated, ``, map[string]string{"Resource-Id": "902"}, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "thread", "note", "7", "--text", "internal FYI")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/conversations/7/notes" {
		t.Fatalf("method/path = %s %s", got.Method, got.Path)
	}
	if decodeBody(t, got.Body)["text"] != "internal FYI" {
		t.Errorf("body = %s", got.Body)
	}
	if decodeBody(t, []byte(stdout))["id"] != "902" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}
