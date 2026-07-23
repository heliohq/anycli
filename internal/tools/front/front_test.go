package front

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMissingTokenFailsExit1(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "", "me")
	if res.result.ExitCode != 1 {
		t.Fatalf("missing token: exit = %d, want 1", res.result.ExitCode)
	}
	if !strings.Contains(res.stderr, "FRONT_TOKEN is not set") {
		t.Fatalf("missing token: stderr = %q, want FRONT_TOKEN message", res.stderr)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "", "me", "--json")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, res.stderr)
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "FRONT_TOKEN") {
		t.Fatalf("json error envelope = %+v", env.Error)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "conversation", "nope")
	if res.result.ExitCode != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", res.result.ExitCode)
	}
}

func TestBareGroupShowsHelpExit0(t *testing.T) {
	res := run(t, "http://127.0.0.1:0", "tok", "conversation")
	if res.result.ExitCode != 0 {
		t.Fatalf("bare group: exit = %d, want 0", res.result.ExitCode)
	}
}

func TestAPIErrorExit1WithJSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {status: 401, body: `{"_error":{"status":401,"title":"unauthorized","message":"bad token"}}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "tok", "me", "--json")
	if res.result.ExitCode != 1 {
		t.Fatalf("api error: exit = %d, want 1", res.result.ExitCode)
	}
	if !res.result.CredentialRejected {
		t.Fatalf("401 should classify as credential-rejected")
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, res.stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 401 {
		t.Fatalf("json api error envelope = %+v", env.Error)
	}
}

func TestBearerAuthAndAcceptHeader(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {status: 200, body: `{"id":"cmp_1","name":"Acme"}`},
	})
	defer srv.Close()

	res := run(t, srv.URL, "sekret", "me")
	if res.result.ExitCode != 0 {
		t.Fatalf("me: exit = %d, want 0 (stderr=%s)", res.result.ExitCode, res.stderr)
	}
	req := findReq(reqs, "GET", "/me")
	if req == nil {
		t.Fatal("no GET /me request recorded")
	}
	if req.Auth != "Bearer sekret" {
		t.Fatalf("Authorization = %q, want Bearer sekret", req.Auth)
	}
	if req.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", req.Accept)
	}
	env := decodeEnvelope(t, res.stdout)
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("me data is not an object: %v", env["data"])
	}
	if data["id"] != "cmp_1" || data["name"] != "Acme" {
		t.Fatalf("me data = %v, want id cmp_1 name Acme", env["data"])
	}
}
