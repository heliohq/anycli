package signnow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWhoami_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user": {http.StatusOK, `{"id":"u1","primary_email":"a@b.com","first_name":"Ada","last_name":"Lovelace"}`},
	})
	defer srv.Close()

	res, stdout, _ := runSN(t, srv, "whoami")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	got := findReq(reqs, http.MethodGet, "/user")
	if got == nil {
		t.Fatalf("no GET /user request recorded: %+v", reqs)
	}
	if got.Auth != "Bearer secret" {
		t.Errorf("Authorization = %q, want Bearer secret", got.Auth)
	}
	out := decodeStdout(t, stdout)
	if out["id"] != "u1" || out["primary_email"] != "a@b.com" {
		t.Errorf("projection = %v, want id/primary_email", out)
	}
}

func TestMissingToken_Exit1(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"whoami"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAccessToken) {
		t.Errorf("stderr = %q, want mention of %s", errBuf.String(), EnvAccessToken)
	}
}

func TestBaseURLFromEnv_Override(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user": {http.StatusOK, `{"id":"u1","primary_email":"a@b.com"}`},
	})
	defer srv.Close()

	// No BaseURL on the struct: the SIGNNOW_API_BASE_URL PROCESS env value must
	// be used (the sandbox-targeting seam), proving the singleton is not mutated
	// and the env override reaches the request path.
	t.Setenv(EnvBaseURL, srv.URL)
	var out, errBuf bytes.Buffer
	svc := &Service{HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"whoami"},
		map[string]string{EnvAccessToken: "secret"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.ExitCode, errBuf.String())
	}
	if svc.BaseURL != "" {
		t.Errorf("singleton BaseURL mutated to %q; env resolution must not touch the shared struct", svc.BaseURL)
	}
	if findReq(reqs, http.MethodGet, "/user") == nil {
		t.Fatalf("request did not reach the env-provided base URL: %+v", reqs)
	}
}

func TestAPIError_CurrentDialect_Exit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /document/doc1": {http.StatusBadRequest, `{"errors":[{"code":42,"message":"bad document"}]}`},
	})
	defer srv.Close()

	res, _, stderr := runSN(t, srv, "document", "get", "doc1")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if res.credentialRejected {
		t.Errorf("a 400 must not reject the credential")
	}
	if !strings.Contains(stderr, "bad document") {
		t.Errorf("stderr = %q, want the errors[] message", stderr)
	}
}

func TestAPIError_LegacyDialect_Exit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /document/doc1": {http.StatusForbidden, `{"error":"access denied"}`},
	})
	defer srv.Close()

	res, _, stderr := runSN(t, srv, "document", "get", "doc1")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(stderr, "access denied") {
		t.Errorf("stderr = %q, want the legacy error string", stderr)
	}
}

func TestAPIError_401_RejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user": {http.StatusUnauthorized, `{"error":"invalid token"}`},
	})
	defer srv.Close()

	res, _, _ := runSN(t, srv, "whoami")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if !res.credentialRejected {
		t.Errorf("a 401 must classify the credential as rejected")
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /document/doc1": {http.StatusBadRequest, `{"errors":[{"code":42,"message":"bad document"}]}`},
	})
	defer srv.Close()

	_, _, stderr := runSN(t, srv, "--json", "document", "get", "doc1")
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
		t.Errorf("envelope = %+v, want kind api / status 400", env.Error)
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	res, _, _ := runSN(t, srv, "document", "frobnicate")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for an unknown subcommand", res.exitCode)
	}
}
