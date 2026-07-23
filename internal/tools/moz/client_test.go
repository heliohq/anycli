package moz

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEnvelopeShape asserts the JSON-RPC 2.0 envelope: method routing, the
// x-moz-token header, JSON content type, POST verb, and an id of at least 24
// characters (a Moz API hard requirement).
func TestEnvelopeShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"jsonrpc":"2.0","id":"x","result":{"quota":{"account_id":42}}}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "quota")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "POST" {
		t.Errorf("HTTP method = %q, want POST", got.Method)
	}
	if got.Token != "moz-tok-123" {
		t.Errorf("x-moz-token = %q, want moz-tok-123", got.Token)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	env := decodeRPC(t, got.Body)
	if env.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", env.JSONRPC)
	}
	if env.Method != "quota.lookup" {
		t.Errorf("method = %q, want quota.lookup", env.Method)
	}
	if len([]rune(env.ID)) < 24 {
		t.Errorf("id %q length = %d, want >= 24", env.ID, len([]rune(env.ID)))
	}
	if !strings.Contains(stdout, `"account_id":42`) {
		t.Errorf("stdout = %q, want the result passthrough", stdout)
	}
}

// TestDefaultRequestIDIsUUIDv4Length asserts the real (non-overridden) id
// generator produces a >= 24-char id — the production path the harness stubs.
func TestDefaultRequestIDIsUUIDv4Length(t *testing.T) {
	s := &Service{}
	id := s.requestID()
	if len([]rune(id)) < 24 {
		t.Errorf("requestID() = %q length %d, want >= 24", id, len([]rune(id)))
	}
	if strings.Count(id, "-") != 4 {
		t.Errorf("requestID() = %q, want a dashed UUIDv4", id)
	}
}

// TestResultPassthrough asserts the provider's result is emitted verbatim with
// a trailing newline (no re-encoding).
func TestResultPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"jsonrpc":"2.0","id":"x","result":{"site_metrics":{"domain_authority":57}}}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "site", "metrics", "--site", "moz.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if stdout != `{"site_metrics":{"domain_authority":57}}`+"\n" {
		t.Errorf("stdout = %q, want the result object verbatim + newline", stdout)
	}
}

// TestJSONRPCErrorMapsToExit1 asserts a JSON-RPC error object becomes an
// apiError (exit 1) surfacing message + explanation.
func TestJSONRPCErrorMapsToExit1(t *testing.T) {
	var got capturedRequest
	body := `{"jsonrpc":"2.0","id":"x","error":{"code":-32655,"status":404,"message":"Not found","data":{"explanation":"That query was not found"}}}`
	srv := newServer(t, 404, body, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "site", "metrics", "--site", "moz.com")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "That query was not found") {
		t.Errorf("stderr = %q, want the explanation", stderr)
	}
}

// TestUnauthorizedRejectsCredential asserts a JSON-RPC 401 error marks the
// credential rejected (the stale-credential feedback loop).
func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	body := `{"jsonrpc":"2.0","id":"x","error":{"code":-32000,"status":401,"message":"Unauthorized","data":{"explanation":"Invalid token"}}}`
	srv := newServer(t, 401, body, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "quota")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", result.ExitCode, stderr)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for a 401")
	}
}

// TestUnauthorizedHTTPWithoutRPCErrorRejectsCredential asserts a bare 401 (no
// JSON-RPC error object) also marks the credential rejected.
func TestUnauthorizedHTTPWithoutRPCErrorRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `Unauthorized`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "quota")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with credential rejection", result)
	}
}

// TestErrorJSONEnvelope asserts the --json error envelope carries kind=api,
// the JSON-RPC code, and the HTTP status.
func TestErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	body := `{"jsonrpc":"2.0","id":"x","error":{"code":-32655,"status":404,"message":"Not found"}}`
	srv := newServer(t, 404, body, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "quota")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Code    int    `json:"code"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Code != -32655 || env.Error.Status != 404 {
		t.Errorf("error envelope = %+v, want kind=api code=-32655 status=404", env.Error)
	}
}

// TestMissingTokenExit1 asserts a missing token fails fast (exit 1) before any
// HTTP call.
func TestMissingTokenExit1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"quota"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MOZ_API_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

// TestUnknownSubcommandExit2 asserts an unknown subcommand is a usage error
// (exit 2), never a false success.
func TestUnknownSubcommandExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "bogus")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for an unknown subcommand", exit)
	}
}
