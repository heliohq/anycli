package keap

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMissingTokenExit1(t *testing.T) {
	res := run(t, nil, "", "contact", "list")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, EnvAccessToken) {
		t.Errorf("stderr = %q, want it to mention %s", res.stderr, EnvAccessToken)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	res := run(t, nil, "", "--json", "contact", "list")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, res.stderr)
	}
	if env.Error.Message == "" {
		t.Errorf("error.message empty in %q", res.stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts": {status: 401, body: `{"message":"token expired"}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "contact", "list")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if !res.rejected {
		t.Errorf("credential was not marked rejected on 401")
	}
	if !strings.Contains(res.stderr, "token expired") {
		t.Errorf("stderr = %q, want provider message surfaced", res.stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts": {status: 422, body: `{"message":"bad filter"}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "--json", "contact", "list")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.stderr), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, res.stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("envelope = %+v, want kind=api status=422", env.Error)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	res := run(t, nil, "tok", "contact", "frobnicate")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for usage error", res.exitCode)
	}
}

func TestBearerAuthInjected(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts": {status: 200, body: `{"contacts":[]}`},
	})
	defer srv.Close()

	res := run(t, srv, "sekret", "contact", "list")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", res.exitCode, res.stderr)
	}
	req := findReq(reqs, "GET", "/v2/contacts")
	if req == nil {
		t.Fatal("no GET /v2/contacts recorded")
	}
	if req.Auth != "Bearer sekret" {
		t.Errorf("Authorization = %q, want %q", req.Auth, "Bearer sekret")
	}
	if strings.TrimSpace(res.stdout) != `{"contacts":[]}` {
		t.Errorf("stdout = %q, want provider JSON passthrough", res.stdout)
	}
}
