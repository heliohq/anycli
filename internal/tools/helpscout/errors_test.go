package helpscout

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingToken(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "HELPSCOUT_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

// TestMissingTokenJSONKind asserts the missing-credential error is classified
// as kind "credential" (exit 1) in the --json envelope — a credential/runtime
// condition, not a flag-usage error.
func TestMissingTokenJSONKind(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"user", "me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if e := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); e != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", e, errBuf.String())
	}
	if env.Error.Kind != "credential" {
		t.Errorf("kind = %q, want credential", env.Error.Kind)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Access token invalid"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "user", "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 must classify as credential rejection")
	}
	if !strings.Contains(stderr, "HTTP 401") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestForbiddenIsRuntimeNotCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"message":"Forbidden"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "user", "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("403 must NOT invalidate the credential")
	}
}

func TestRateLimitSurfacesRetryAfter(t *testing.T) {
	srv := newHeaderServer(t, http.StatusTooManyRequests, `{"message":"Too Many Requests"}`,
		map[string]string{"X-RateLimit-Retry-After": "12"}, &capturedRequest{})
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "conversation", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("429 must not invalidate the credential")
	}
	if !strings.Contains(stderr, "retry after 12s") {
		t.Errorf("stderr = %q, want retry-after hint", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"The mailbox is required"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "conversation", "list", "--json")
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
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind=api status=400", env.Error)
	}
	if !strings.Contains(env.Error.Message, "mailbox is required") {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestUnknownSubcommandIsUsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "conversation", "bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("unknown subcommand must not hit the API")
	}
}
