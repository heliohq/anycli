package onesignal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingAppAPIKey_Exit1(t *testing.T) {
	result, _, stderr := runNoServer(t, map[string]string{EnvAppID: testAppID}, "message", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, EnvAppAPIKey) {
		t.Errorf("stderr = %q, want mention of %s", stderr, EnvAppAPIKey)
	}
}

func TestExecute_MissingAppID_Exit1(t *testing.T) {
	result, _, stderr := runNoServer(t, map[string]string{EnvAppAPIKey: testKey}, "message", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, EnvAppID) {
		t.Errorf("stderr = %q, want mention of %s", stderr, EnvAppID)
	}
}

func TestExecute_MissingCredential_JSONKindMatchesExitCode(t *testing.T) {
	// The missing-credential branch is exit 1, so its --json envelope must carry
	// kind "api" — exit 1 pairs with "api" everywhere else in this tool, never
	// "usage" (which is reserved for exit 2).
	result, _, stderr := runNoServer(t, map[string]string{EnvAppID: testAppID}, "message", "list", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api (to match exit 1)", env.Error.Kind)
	}
}

func TestExecute_Unauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"errors":["Invalid app API key"]}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "message", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("expected CredentialRejected on 401")
	}
}

func TestExecute_APIError400_Exit1NotRejected(t *testing.T) {
	srv := newServer(t, http.StatusBadRequest, `{"errors":["app_id not found"]}`, &capturedRequest{})
	defer srv.Close()

	result, _, _ := runResult(t, srv, "message", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("400 should not reject the credential")
	}
}

func TestExecute_JSONErrorEnvelope_APIError(t *testing.T) {
	srv := newServer(t, http.StatusBadRequest, `{"errors":["bad request"]}`, &capturedRequest{})
	defer srv.Close()

	_, _, stderr := run(t, srv, "message", "list", "--json")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusBadRequest {
		t.Errorf("envelope = %+v, want kind=api status=400", env.Error)
	}
}

func TestExecute_JSONErrorEnvelope_Usage(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "message", "send", "--content", "hi", "--json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	var env struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestExecute_UnknownSubcommand_Exit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected for unknown subcommand")
	}
}
