package jotform

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A read-only key's 401 on a write is surfaced verbatim as a runtime error
// (exit 1) — and must NOT be flagged as a rejected credential, since the key is
// still valid for reads (Jotform overloads 401 for permission vs invalid key).
func TestReadOnlyKey401_Exit1_NotRejected(t *testing.T) {
	const body = `{"responseCode":401,"message":"You're not authorized to use create submission action.","content":""}`
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, body, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "submission", "create", "f1", "--field", "3=x")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("401 must not mark the credential rejected (read-only key stays valid for reads)")
	}
	if !strings.Contains(stderr, "You're not authorized") {
		t.Errorf("stderr = %q, want Jotform message surfaced verbatim", stderr)
	}
	if !strings.Contains(stderr, "401") {
		t.Errorf("stderr = %q, want the HTTP status surfaced", stderr)
	}
}

// Jotform sometimes returns HTTP 200 with an error responseCode; that is still
// an API failure.
func TestErrorEnvelopeInside200_Exit1(t *testing.T) {
	const body = `{"responseCode":404,"message":"Form not found","content":""}`
	var got capturedRequest
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	result, stdout, stderr := runResult(t, srv, "form", "get", "999")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "404") || !strings.Contains(stderr, "Form not found") {
		t.Errorf("stderr = %q, want status 404 + message", stderr)
	}
}

func TestError_JSONEnvelope(t *testing.T) {
	const body = `{"responseCode":401,"message":"Invalid API key","content":""}`
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, body, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "user", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != 401 {
		t.Errorf("status = %d, want 401", env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "Invalid API key") {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "bogus")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", code)
	}
	if got.Method != "" {
		t.Errorf("unknown subcommand must not hit the API, saw %s", got.Method)
	}
}

// encodeFields is the sole write-path encoder; cover its branches directly.
func TestEncodeFields(t *testing.T) {
	form, err := encodeFields([]string{"3=simple", "10:area=NY", "7=with spaces"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.Get("submission[3]") != "simple" {
		t.Errorf("submission[3] = %q", form.Get("submission[3]"))
	}
	if form.Get("submission[10][area]") != "NY" {
		t.Errorf("submission[10][area] = %q", form.Get("submission[10][area]"))
	}
	if form.Get("submission[7]") != "with spaces" {
		t.Errorf("submission[7] = %q", form.Get("submission[7]"))
	}

	for _, bad := range []string{"noequals", "=novalue", "3:=x"} {
		if _, err := encodeFields([]string{bad}); err == nil {
			t.Errorf("encodeFields(%q) = nil error, want usage error", bad)
		}
	}
}
