package courier

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingAPIKeyExits1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"message", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "COURIER_API_KEY") {
		t.Fatalf("stderr = %q, want mention of COURIER_API_KEY", errBuf.String())
	}
}

func TestMissingAPIKeyJSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"--json", "message", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if e := json.Unmarshal([]byte(errBuf.String()), &env); e != nil {
		t.Fatalf("stderr not JSON envelope: %v (%q)", e, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Fatalf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestUnknownSubcommandExits2(t *testing.T) {
	code, _, _ := run(t, nil, "message", "bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", code)
	}
}

func TestBareGroupShowsHelpExit0(t *testing.T) {
	// A runnable group with no subcommand prints help and exits 0 — but never
	// makes a network call. nil server proves no HTTP happens.
	code, _, _ := run(t, nil, "message")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for bare group help", code)
	}
}

func TestAPIErrorExit1AndStatusEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusInternalServerError, `{"message":"boom"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "message", "get", "req-1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if e := json.Unmarshal([]byte(stderr), &env); e != nil {
		t.Fatalf("stderr not JSON envelope: %v (%q)", e, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 500 {
		t.Fatalf("envelope = %+v, want kind=api status=500", env.Error)
	}
}

func Test401RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "message", "get", "req-1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("CredentialRejected = false, want true on 401")
	}
}

func TestBearerAuthAndAcceptHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Auth != "Bearer key-123" {
		t.Fatalf("Authorization = %q, want Bearer key-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got.Accept)
	}
}

func TestPassthroughStdout(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":"x"}],"paging":{"more":false}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "message", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, `"results"`) || !strings.Contains(stdout, `"paging"`) {
		t.Fatalf("stdout = %q, want raw provider JSON passthrough", stdout)
	}
}
