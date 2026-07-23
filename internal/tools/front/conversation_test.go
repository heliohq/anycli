package front

import (
	"testing"
)

const listBody = `{
  "_pagination": {"next": "https://acme.api.frontapp.com/conversations?limit=2&page_token=CURSOR123"},
  "_results": [
    {"id": "cnv_1", "subject": "Refund?"},
    {"id": "cnv_2", "subject": "Where is my order"}
  ]
}`

func TestConversationListNormalizesEnvelopeAndCursor(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /conversations": {status: 200, body: listBody},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "list", "--limit", "2")
	if res.result.ExitCode != 0 {
		t.Fatalf("list: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "GET", "/conversations")
	if req == nil {
		t.Fatal("no GET /conversations request")
	}
	if got := req.Query["limit"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("limit query = %v, want [2]", req.Query["limit"])
	}
	env := decodeEnvelope(t, res.stdout)
	data, ok := env["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("data = %v, want 2 items", env["data"])
	}
	if env["next_page_token"] != "CURSOR123" {
		t.Fatalf("next_page_token = %v, want CURSOR123 (lifted from _pagination.next)", env["next_page_token"])
	}
}

func TestConversationListEmptyEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /conversations": {status: 200, body: `{"_pagination":{"next":null},"_results":[]}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "list")
	env := decodeEnvelope(t, res.stdout)
	data, ok := env["data"].([]any)
	if !ok || len(data) != 0 {
		t.Fatalf("data = %v, want empty array", env["data"])
	}
	if env["next_page_token"] != "" {
		t.Fatalf("next_page_token = %v, want empty at end of results", env["next_page_token"])
	}
}

func TestConversationListSearchRouting(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /conversations/search/refund": {status: 200, body: `{"_results":[]}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "list", "--q", "refund")
	if res.result.ExitCode != 0 {
		t.Fatalf("search: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	if findReq(reqs, "GET", "/conversations/search/refund") == nil {
		t.Fatalf("--q did not route to the search endpoint; paths=%v", reqs)
	}
}

func TestConversationListInboxRouting(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /inboxes/inb_9/conversations": {status: 200, body: `{"_results":[]}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "list", "--inbox", "inb_9")
	if res.result.ExitCode != 0 {
		t.Fatalf("inbox list: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	if findReq(reqs, "GET", "/inboxes/inb_9/conversations") == nil {
		t.Fatal("--inbox did not route to the inbox conversations endpoint")
	}
}

func TestConversationListBadSortOrderExit2(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "conversation", "list", "--sort-order", "sideways")
	if res.result.ExitCode != 2 {
		t.Fatalf("bad sort-order: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestConversationGetRequiresID(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "conversation", "get")
	if res.result.ExitCode != 2 {
		t.Fatalf("missing --id: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestConversationMessagesEmitsList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /conversations/cnv_1/messages": {status: 200, body: `{"_results":[{"id":"msg_1"}]}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "messages", "--id", "cnv_1")
	if res.result.ExitCode != 0 {
		t.Fatalf("messages: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if data, ok := env["data"].([]any); !ok || len(data) != 1 {
		t.Fatalf("messages data = %v, want 1 item", env["data"])
	}
}

func TestConversationUpdateIssuesDistinctCalls(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /conversations/cnv_1":        {status: 204, body: ``},
		"PUT /conversations/cnv_1/assignee": {status: 204, body: ``},
		"POST /conversations/cnv_1/tags":    {status: 204, body: ``},
		"DELETE /conversations/cnv_1/tags":  {status: 204, body: ``},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "update", "--id", "cnv_1",
		"--status", "archived", "--assignee", "tea_5", "--tag-add", "tag_a", "--tag-remove", "tag_b")
	if res.result.ExitCode != 0 {
		t.Fatalf("update: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	patch := findReq(reqs, "PATCH", "/conversations/cnv_1")
	if patch == nil || bodyMap(t, patch.Body)["status"] != "archived" {
		t.Fatalf("PATCH body = %v, want status archived", patch)
	}
	assign := findReq(reqs, "PUT", "/conversations/cnv_1/assignee")
	if assign == nil || bodyMap(t, assign.Body)["assignee_id"] != "tea_5" {
		t.Fatalf("assignee body = %v, want assignee_id tea_5", assign)
	}
	if findReq(reqs, "POST", "/conversations/cnv_1/tags") == nil {
		t.Fatal("no POST tags (tag-add)")
	}
	if findReq(reqs, "DELETE", "/conversations/cnv_1/tags") == nil {
		t.Fatal("no DELETE tags (tag-remove)")
	}
}

func TestConversationUpdateUnassignSendsNull(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /conversations/cnv_1/assignee": {status: 204, body: ``},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "conversation", "update", "--id", "cnv_1", "--assignee", "null")
	if res.result.ExitCode != 0 {
		t.Fatalf("unassign: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	assign := findReq(reqs, "PUT", "/conversations/cnv_1/assignee")
	if assign == nil {
		t.Fatal("no assignee request")
	}
	body := bodyMap(t, assign.Body)
	if v, present := body["assignee_id"]; !present || v != nil {
		t.Fatalf("assignee_id = %v (present=%v), want JSON null", v, present)
	}
}

func TestConversationUpdateNoChangeExit2(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "conversation", "update", "--id", "cnv_1")
	if res.result.ExitCode != 2 {
		t.Fatalf("no-op update: exit = %d, want 2", res.result.ExitCode)
	}
}
