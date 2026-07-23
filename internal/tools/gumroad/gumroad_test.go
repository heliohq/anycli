package gumroad

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserGetPassesBearerAndEmitsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"user":{"user_id":"u1","name":"Ada","email":"ada@x.io"}}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "user", "get")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/v2/user" {
		t.Fatalf("request = %s %s, want GET /v2/user", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Fatalf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	// Passthrough: full body preserved (including the success wrapper).
	v := decodeJSON(t, stdout).(map[string]any)
	if v["success"] != true {
		t.Fatalf("stdout missing success:true: %s", stdout)
	}
}

func TestMissingTokenExitOne(t *testing.T) {
	result, _, stderr := runNoToken(t, "user", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "GUMROAD_ACCESS_TOKEN") {
		t.Fatalf("stderr = %q, want mention of GUMROAD_ACCESS_TOKEN", stderr)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	result, _, stderr := runNoToken(t, "user", "get", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Message == "" {
		t.Fatalf("empty error message: %s", stderr)
	}
}

// The 200-with-success:false dialect: HTTP 200 but success:false is an API
// error, not a passthrough success. Exit 1.
func TestSuccessFalseOn200IsError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":false,"message":"The product was not found."}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "product", "get", "--id", "nope")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (stdout=%q stderr=%q)", exit, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout should be empty on error, got %q", stdout)
	}
	if !strings.Contains(stderr, "The product was not found.") {
		t.Fatalf("stderr = %q, want Gumroad message", stderr)
	}
}

func TestSuccessFalseJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":false,"message":"boom"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "product", "get", "--id", "nope", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Message != "boom" {
		t.Fatalf("envelope = %+v, want api/boom", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"success":false,"message":"Unauthorized"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "user", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("CredentialRejected = false, want true on HTTP 401 (stderr=%q)", stderr)
	}
}

func TestNon2xxWithoutSuccessFieldIsError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"success":false,"message":"Not Found"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "sale", "get", "--id", "x")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatalf("404 must not reject the credential")
	}
}

func TestUsageErrorExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	// Missing required --id.
	result, _, _ := runResult(t, srv, "product", "get")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing required flag", result.ExitCode)
	}
}

func TestUnknownSubcommandExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "bogus")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", result.ExitCode)
	}
}
