package pinterest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccountGetInjectsBearerAndPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"123","username":"acme"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "account", "get")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/user_account" {
		t.Errorf("request = %s %s, want GET /user_account", got.Method, got.Path)
	}
	if got.Auth != "Bearer pina-tok-123" {
		t.Errorf("auth = %q, want Bearer pina-tok-123", got.Auth)
	}
	if !strings.Contains(stdout, `"username":"acme"`) {
		t.Errorf("stdout = %q, want passthrough JSON", stdout)
	}
}

func TestMissingTokenExitsOne(t *testing.T) {
	svc := &Service{}
	// No BaseURL/HC: must fail before any network call.
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(t.Context(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PINTEREST_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestUnknownSubcommandIsUsageExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "board", "frobnicate")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (usage), stderr=%q", exit, stderr)
	}
}

func TestMissingRequiredFlagIsUsageExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "board", "create")
	if exit != 2 {
		t.Errorf("exit = %d, want 2, stderr=%q", exit, stderr)
	}
	if !strings.Contains(stderr, "--name is required") {
		t.Errorf("stderr = %q, want --name required", stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should be made, saw %s %s", got.Method, got.Path)
	}
}

func TestUnauthorizedRejectsCredentialWithReconnectHint(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"code":2,"message":"Authentication failed"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account", "get")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 should classify as credential rejected (reconnect)")
	}
	if !strings.Contains(stderr, "reconnect") {
		t.Errorf("stderr = %q, want reconnect hint", stderr)
	}
	if !strings.Contains(stderr, "Authentication failed") {
		t.Errorf("stderr = %q, want provider message", stderr)
	}
}

func TestRateLimitSurfacesBackoffHintNotRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"code":8,"message":"You have exceeded your rate limit"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "pin", "list")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("429 must NOT reject the credential")
	}
	if !strings.Contains(stderr, "back off") {
		t.Errorf("stderr = %q, want backoff hint", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"code":1,"message":"invalid parameters"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "pin", "get", "abc")
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
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind api status 400", env.Error)
	}
	if !strings.Contains(env.Error.Message, "invalid parameters") {
		t.Errorf("message = %q, want provider text", env.Error.Message)
	}
}
