package pandadoc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExecute_BearerAuthSelected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "template", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Auth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q, want Bearer tok-abc", got.Auth)
	}
}

func TestExecute_APIKeyAuthPreferred(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"results":[]}`, &got)
	defer srv.Close()

	// Both credentials present: the API-Key scheme wins (L2 dev convenience).
	env := map[string]string{EnvAccessToken: "tok-abc", EnvAPIKey: "key-xyz"}
	result, _, stderr := runEnv(t, srv, env, "template", "list")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	if got.Auth != "API-Key key-xyz" {
		t.Errorf("Authorization = %q, want API-Key key-xyz", got.Auth)
	}
}

func TestExecute_MissingCredentials(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"template", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PANDADOC_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-credential message", errBuf.String())
	}
}

func TestExecute_MissingCredentials_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"--json", "template", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("error.kind = %q, want usage", env.Error.Kind)
	}
}

func TestWhoami(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"user_id":"u-1","email":"a@acme.com","first_name":"A","last_name":"B"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/public/v1/members/current" {
		t.Errorf("request = %s %s, want GET /public/v1/members/current", got.Method, got.Path)
	}
	if !strings.Contains(stdout, "a@acme.com") || !strings.Contains(stdout, "u-1") {
		t.Errorf("stdout = %q, want email and user_id", stdout)
	}
}

func TestWhoami_JSONPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"user_id":"u-1","email":"a@acme.com"}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "whoami", "--json")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	if m["user_id"] != "u-1" {
		t.Errorf("stdout user_id = %v, want u-1", m["user_id"])
	}
}

func TestAPIError_ExitAndEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 400, `{"type":"validation_error","detail":"bad template_uuid"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "template", "list", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("error = %+v, want kind=api status=400", env.Error)
	}
}

func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"type":"authentication_error","detail":"invalid token"}`, &got)
	defer srv.Close()

	result, _, _ := runEnv(t, srv, map[string]string{EnvAccessToken: "tok-abc"}, "template", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on 401")
	}
}

func TestUsageError_ExitTwo(t *testing.T) {
	// Unknown subcommand is a cobra parse error → exit 2.
	result, _, _ := runEnv(t, nil, map[string]string{EnvAccessToken: "tok-abc"}, "bogus")
	if result.ExitCode != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", result.ExitCode)
	}
}
