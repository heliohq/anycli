package apollo

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingTokenFailsWithExitOne(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(context.Background(), []string{"people", "enrich", "--email", "a@b.com"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "APOLLO_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestMissingTokenJSONError(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	_, err := svc.Execute(context.Background(), []string{"--json", "users", "profile"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if e := json.Unmarshal([]byte(errBuf.String()), &env); e != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", e, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "APOLLO_ACCESS_TOKEN") {
		t.Fatalf("json error = %+v, want usage/missing-token", env.Error)
	}
}

func TestBearerAuthAndAcceptHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"user":{"id":"u1"}}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "users", "profile")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Auth != "Bearer tok-123" {
		t.Fatalf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got.Accept)
	}
	if got.Method != http.MethodGet || got.Path != "/users/api_profile" {
		t.Fatalf("request = %s %s, want GET /users/api_profile", got.Method, got.Path)
	}
	if strings.TrimSpace(stdout) != `{"user":{"id":"u1"}}` {
		t.Fatalf("stdout = %q, want verbatim provider JSON", stdout)
	}
}

func TestUnknownSubcommandIsUsageErrorExitTwo(t *testing.T) {
	exit, _, _ := run(t, nil, "people", "nope")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}

func TestBareGroupShowsHelpExitZero(t *testing.T) {
	// A runnable group prints help and exits 0 (not a false-success on an
	// unknown leaf).
	exit, _, _ := run(t, nil, "people")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 for bare group help", exit)
	}
}

func TestAPIErrorRendersPlainAndExitsOne(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"error":"invalid filters"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contacts", "search", "--q", "x")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "invalid filters") {
		t.Fatalf("stderr = %q, want Apollo error message", stderr)
	}
}

func TestAPIErrorRendersJSONEnvelopeWithStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"error":"master key required"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "people", "search", "--title", "cto")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if e := json.Unmarshal([]byte(stderr), &env); e != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", e, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusForbidden {
		t.Fatalf("json error = %+v, want api/403", env.Error)
	}
	if !strings.Contains(env.Error.Message, "master key") {
		t.Fatalf("message = %q, want master-key access hint", env.Error.Message)
	}
}

func TestCredentialRejectedOn401(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "users", "profile")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("CredentialRejected = false, want true on 401")
	}
}
