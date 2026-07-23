package front

import (
	"testing"
)

func TestMessageSendBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /conversations/cnv_1/messages": {status: 202, body: `{"id":"msg_9","type":"email"}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "message", "send", "--conversation", "cnv_1",
		"--body", "Thanks, refunded.", "--text", "Thanks, refunded.", "--author", "tea_1")
	if res.result.ExitCode != 0 {
		t.Fatalf("send: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/conversations/cnv_1/messages")
	if req == nil {
		t.Fatal("no POST messages request")
	}
	if req.ContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["body"] != "Thanks, refunded." || body["text"] != "Thanks, refunded." || body["author_id"] != "tea_1" {
		t.Fatalf("send body = %v", body)
	}
	env := decodeEnvelope(t, res.stdout)
	if data, ok := env["data"].(map[string]any); !ok || data["id"] != "msg_9" {
		t.Fatalf("send data = %v, want id msg_9", env["data"])
	}
}

func TestMessageSendRequiresConversationAndBody(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "message", "send", "--conversation", "cnv_1")
	if res.result.ExitCode != 2 {
		t.Fatalf("missing --body: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestDraftCreateRequiresChannel(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "draft", "create", "--conversation", "cnv_1", "--body", "hi")
	if res.result.ExitCode != 2 {
		t.Fatalf("missing --channel: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestDraftCreateBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /conversations/cnv_1/drafts": {status: 200, body: `{"id":"msg_d","is_draft":true}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "draft", "create", "--conversation", "cnv_1",
		"--body", "Draft reply", "--channel", "cha_2")
	if res.result.ExitCode != 0 {
		t.Fatalf("draft: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/conversations/cnv_1/drafts")
	if req == nil {
		t.Fatal("no POST drafts request")
	}
	body := bodyMap(t, req.Body)
	if body["body"] != "Draft reply" || body["channel_id"] != "cha_2" {
		t.Fatalf("draft body = %v, want body+channel_id", body)
	}
}

func TestCommentAdd(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /conversations/cnv_1/comments": {status: 201, body: `{"id":"com_1"}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "comment", "add", "--conversation", "cnv_1",
		"--body", "@teammate please check", "--author", "tea_3")
	if res.result.ExitCode != 0 {
		t.Fatalf("comment: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/conversations/cnv_1/comments")
	if req == nil {
		t.Fatal("no POST comments request")
	}
	body := bodyMap(t, req.Body)
	if body["body"] != "@teammate please check" || body["author_id"] != "tea_3" {
		t.Fatalf("comment body = %v", body)
	}
}
