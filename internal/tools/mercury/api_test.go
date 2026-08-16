package mercury

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

func TestAPI_GetVerbatim(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/credit": {status: 200, body: `{"accounts":[{"id":"cr1"}]}`},
	})
	defer srv.Close()

	code, out, _ := run(t, srv, "api", "get", "/credit")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "GET", "/api/v1/credit")
	if req == nil {
		t.Fatal("no GET /api/v1/credit request")
	}
	if req.Auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token (no secret-token: prefix)", req.Auth)
	}
	// The escape hatch emits the provider body verbatim — no {"data":...} envelope.
	if strings.TrimSpace(out) != `{"accounts":[{"id":"cr1"}]}` {
		t.Errorf("stdout = %q, want the verbatim provider body", out)
	}
}

func TestAPI_PathNormalization(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"bare path without slash", "accounts"},
		{"path with /api/v1 prefix", "/api/v1/accounts"},
		{"full URL with prefix and query", "https://api.mercury.com/api/v1/accounts?limit=5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				"GET /api/v1/accounts": {status: 200, body: `{"accounts":[]}`},
			})
			defer srv.Close()
			code, _, _ := run(t, srv, "api", "GET", tc.path)
			if code != 0 {
				t.Fatalf("exit = %d, want 0", code)
			}
			req := findReq(reqs, "GET", "/api/v1/accounts")
			if req == nil {
				t.Fatalf("no GET /api/v1/accounts request for path %q; reqs=%v", tc.path, reqs)
			}
			if strings.Contains(tc.path, "limit=5") {
				if got := req.Query["limit"]; len(got) != 1 || got[0] != "5" {
					t.Errorf("limit query = %v, want [5]", got)
				}
			}
		})
	}
}

func TestAPI_PostBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /api/v1/things": {status: 200, body: `{"ok":true}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "api", "POST", "/things", "--body", `{"name":"x"}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "POST", "/api/v1/things")
	if req == nil {
		t.Fatal("no POST /api/v1/things request")
	}
	if req.Body != `{"name":"x"}` {
		t.Errorf("body = %q, want the raw --body payload", req.Body)
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json default for a body", req.ContentType)
	}
}

func TestAPI_BodyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /api/v1/things": {status: 200, body: `{"ok":true}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "api", "POST", "/things", "--body-file", path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "POST", "/api/v1/things")
	if req == nil || req.Body != `{"from":"file"}` {
		t.Fatalf("request = %+v, want body from file", req)
	}
}

func TestAPI_BodyAndBodyFileMutuallyExclusive(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, errb := run(t, srv, "api", "POST", "/things", "--body", `{}`, "--body-file", "x.json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made, got %d", len(reqs))
	}
	if !strings.Contains(errb, "mutually exclusive") {
		t.Errorf("stderr = %q, want mutual-exclusion message", errb)
	}
}

func TestAPI_CustomHeader(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 200, body: `{"accounts":[]}`},
	})
	defer srv.Close()
	code, _, _ := run(t, srv, "api", "GET", "/accounts", "--header", "X-Custom: yes")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "GET", "/api/v1/accounts")
	if req == nil {
		t.Fatal("no request")
	}
	if got := req.Headers.Get("X-Custom"); got != "yes" {
		t.Errorf("X-Custom header = %q, want yes", got)
	}
}

func TestAPI_RejectsAuthHeaderOverride(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, errb := run(t, srv, "api", "GET", "/accounts", "--header", "Authorization: nope")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made, got %d", len(reqs))
	}
	if !strings.Contains(errb, "cannot be overridden") {
		t.Errorf("stderr = %q, want override rejection", errb)
	}
}

func TestAPI_ErrorExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/nope": {status: 500, body: `{"message":"boom"}`},
	})
	defer srv.Close()
	code, _, errb := run(t, srv, "api", "GET", "/nope")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb, "boom") || !strings.Contains(errb, "500") {
		t.Errorf("stderr = %q, want Mercury message + status", errb)
	}
}

func TestAPI_CredentialRejectedOn401(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 401, body: `{"message":"unauthorized"}`},
	})
	defer srv.Close()
	var out, errb bytes.Buffer
	svc := &Service{FS: execution.OS{}, BaseURL: srv.URL + "/api/v1", HC: srv.Client(), Out: &out, Err: &errb}
	res, _ := svc.Execute(context.Background(), []string{"api", "GET", "/accounts"}, map[string]string{EnvToken: "bad"})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("CredentialRejected = false, want true so the token gateway refreshes")
	}
}
