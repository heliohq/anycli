package typeform

import (
	"net/http"
	"strings"
	"testing"
)

func TestWebhookList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1/webhooks": {status: 200, body: `{"items":[]}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "webhook", "list", "f1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/forms/f1/webhooks") == nil {
		t.Fatal("no GET /forms/f1/webhooks recorded")
	}
}

func TestWebhookGetByTag(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1/webhooks/mytag": {status: 200, body: `{"tag":"mytag"}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "webhook", "get", "f1", "mytag")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/forms/f1/webhooks/mytag") == nil {
		t.Fatal("no GET /forms/f1/webhooks/mytag recorded")
	}
}

func TestWebhookSetPutsBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /forms/f1/webhooks/mytag": {status: 200, body: `{"tag":"mytag","enabled":true}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "webhook", "set", "f1", "mytag",
		"--url", "https://example.com/hook", "--enabled", "--verify-ssl",
		"--secret", "s3cr3t", "--event-types", `{"form_response_partial":true}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodPut, "/forms/f1/webhooks/mytag")
	if req == nil {
		t.Fatal("no PUT recorded")
	}
	body := bodyMap(t, req.Body)
	if body["url"] != "https://example.com/hook" {
		t.Errorf("url = %v", body["url"])
	}
	if body["enabled"] != true {
		t.Errorf("enabled = %v, want true", body["enabled"])
	}
	if body["verify_ssl"] != true {
		t.Errorf("verify_ssl = %v, want true", body["verify_ssl"])
	}
	if body["secret"] != "s3cr3t" {
		t.Errorf("secret = %v", body["secret"])
	}
	et, ok := body["event_types"].(map[string]any)
	if !ok || et["form_response_partial"] != true {
		t.Errorf("event_types = %v, want object with form_response_partial=true", body["event_types"])
	}
}

func TestWebhookSetOmitsUnsetBooleans(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /forms/f1/webhooks/t": {status: 200, body: `{}`},
	})
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "webhook", "set", "f1", "t", "--url", "https://x.co/h")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodPut, "/forms/f1/webhooks/t")
	body := bodyMap(t, req.Body)
	if _, present := body["enabled"]; present {
		t.Error("enabled sent when flag unset; should be omitted so the API keeps its default")
	}
	if _, present := body["verify_ssl"]; present {
		t.Error("verify_ssl sent when flag unset; should be omitted")
	}
}

func TestWebhookSetRequiresURL(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "webhook", "set", "f1", "t")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 without --url", exit)
	}
	if !strings.Contains(strings.ToLower(stderr), "url") {
		t.Errorf("stderr = %q, want required-url message", stderr)
	}
}

func TestWebhookDeleteSends204Receipt(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"DELETE /forms/f1/webhooks/t": {status: 204, body: ``},
	})
	defer srv.Close()

	stdout, _, exit := run(t, srv, "tok", "webhook", "delete", "f1", "t")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodDelete, "/forms/f1/webhooks/t") == nil {
		t.Fatal("no DELETE recorded")
	}
	if !strings.Contains(stdout, `"deleted":true`) {
		t.Errorf("stdout = %q, want deleted receipt", stdout)
	}
}
