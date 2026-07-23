package front

import (
	"testing"
)

func TestContactGetByAlias(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /contacts/alt:email:jane@example.com": {status: 200, body: `{"id":"crd_1","name":"Jane"}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "contact", "get", "--id", "alt:email:jane@example.com")
	if res.result.ExitCode != 0 {
		t.Fatalf("contact get: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	if findReq(reqs, "GET", "/contacts/alt:email:jane@example.com") == nil {
		t.Fatalf("alias not passed through; paths=%v", reqs)
	}
}

func TestContactCreateHandles(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /contacts": {status: 201, body: `{"id":"crd_2"}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "contact", "create", "--name", "Jane",
		"--handle", "email:jane@example.com", "--handle", "phone:+15551234567")
	if res.result.ExitCode != 0 {
		t.Fatalf("contact create: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/contacts")
	if req == nil {
		t.Fatal("no POST contacts request")
	}
	body := bodyMap(t, req.Body)
	handles, ok := body["handles"].([]any)
	if !ok || len(handles) != 2 {
		t.Fatalf("handles = %v, want 2", body["handles"])
	}
	first, _ := handles[0].(map[string]any)
	if first["source"] != "email" || first["handle"] != "jane@example.com" {
		t.Fatalf("first handle = %v, want source email handle jane@example.com", first)
	}
	if body["name"] != "Jane" {
		t.Fatalf("name = %v, want Jane", body["name"])
	}
}

func TestContactCreateBadHandleExit2(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "contact", "create", "--handle", "noColonHere")
	if res.result.ExitCode != 2 {
		t.Fatalf("bad handle: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestContactCreateRequiresHandle(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "contact", "create", "--name", "Jane")
	if res.result.ExitCode != 2 {
		t.Fatalf("missing --handle: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestDirectoryListsEmitEnvelope(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"inbox", "list"}, "/inboxes"},
		{[]string{"teammate", "list"}, "/teammates"},
		{[]string{"tag", "list"}, "/tags"},
		{[]string{"contact", "list"}, "/contacts"},
	}
	for _, tc := range cases {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"GET " + tc.path: {status: 200, body: `{"_results":[{"id":"x"}],"_pagination":{"next":null}}`},
		})
		res := run(t, srv.URL, "tok", tc.args...)
		if res.result.ExitCode != 0 {
			t.Fatalf("%v: exit = %d, want 0 (stderr=%s)", tc.args, res.result.ExitCode, res.stderr)
		}
		env := decodeEnvelope(t, res.stdout)
		if data, ok := env["data"].([]any); !ok || len(data) != 1 {
			t.Fatalf("%v: data = %v, want 1 item", tc.args, env["data"])
		}
		srv.Close()
	}
}
