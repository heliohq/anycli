package gorgias

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAccountGetSendsBearerAndAccept(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":42,"name":"Acme"}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/account" {
		t.Errorf("request = %s %s, want GET /account", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if strings.TrimSpace(stdout) != `{"id":42,"name":"Acme"}` {
		t.Errorf("stdout = %q, want the account JSON passthrough", stdout)
	}
}

// TestBaseURLBuiltFromSubdomain proves that, with no BaseURL override, the
// service targets https://{subdomain}.gorgias.com/api — the single most
// important fact about the integration.
func TestBaseURLBuiltFromSubdomain(t *testing.T) {
	svc := &Service{}
	got := svc.resolveBaseURL("", "green-garden")
	if got != "https://green-garden.gorgias.com/api" {
		t.Errorf("resolveBaseURL = %q, want https://green-garden.gorgias.com/api", got)
	}
	// An explicit BaseURL override (tests) wins and is trailing-slash trimmed.
	if got := svc.resolveBaseURL("http://127.0.0.1:9/", "ignored"); got != "http://127.0.0.1:9" {
		t.Errorf("resolveBaseURL override = %q, want trimmed base", got)
	}
}

func TestMissingTokenIsRuntimeFailure(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{EnvSubdomain: "acme"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "GORGIAS_ACCESS_TOKEN") {
		t.Errorf("stderr = %q, want a GORGIAS_ACCESS_TOKEN hint", errBuf.String())
	}
}

func TestMissingSubdomainIsRuntimeFailure(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{EnvAccessToken: "tok"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "GORGIAS_SUBDOMAIN") {
		t.Errorf("stderr = %q, want a GORGIAS_SUBDOMAIN hint", errBuf.String())
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "ticket", "bogus")
	if code != 2 {
		t.Errorf("exit = %d, want 2 (usage)", code)
	}
	if got.Method != "" {
		t.Errorf("unexpected API call for a usage error: %s %s", got.Method, got.Path)
	}
}

func TestAPIErrorIsExit1WithMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"error":{"message":"Ticket not found","type":"NotFound"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "get", "999")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Ticket not found") {
		t.Errorf("stderr = %q, want the Gorgias error message", stderr)
	}
	if !strings.Contains(stderr, "404") {
		t.Errorf("stderr = %q, want the HTTP status", stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"error":"invalid or expired API token"}`, &got)
	defer srv.Close()

	res, _, stderr := runResult(t, srv, "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("a 401 should mark the credential rejected")
	}
	if !strings.Contains(stderr, "invalid or expired API token") {
		t.Errorf("stderr = %q, want the Gorgias error message", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"error":{"message":"nope"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "ticket", "get", "1", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Errorf("envelope = %+v, want kind api / status 404", env.Error)
	}
	if !strings.Contains(env.Error.Message, "nope") {
		t.Errorf("envelope message = %q, want the provider message", env.Error.Message)
	}
}
