package salesloft

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMe(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/me": {status: http.StatusOK, body: `{"data":{"guid":"g-1","email":"rep@example.com"}}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/me")
	if req == nil {
		t.Fatal("expected GET /v2/me")
	}
	if req.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", req.Auth)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
	if !strings.Contains(stdout, `"guid":"g-1"`) {
		t.Errorf("stdout = %q, want passthrough envelope", stdout)
	}
}

func TestMissingTokenExitOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "SALESLOFT_ACCESS_TOKEN") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestUnknownSubcommandExitTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, _ := run(t, srv, "bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for unknown command", code)
	}
}

func TestErrorString403PlainAndJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/people/9": {status: http.StatusForbidden, body: `{"error":"You do not have access"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "get", "--id", "9")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "You do not have access") {
		t.Errorf("stderr = %q, want the error string", stderr)
	}

	// --json path renders the api envelope carrying the HTTP status.
	var reqs2 []capturedRequest
	srv2 := newMux(t, &reqs2, map[string]stub{
		"GET /v2/people/9": {status: http.StatusForbidden, body: `{"error":"You do not have access"}`},
	})
	defer srv2.Close()
	_, _, stderr2 := run(t, srv2, "person", "get", "--id", "9", "--json")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr2), &env); err != nil {
		t.Fatalf("stderr not JSON: %v (%s)", err, stderr2)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusForbidden {
		t.Errorf("envelope = %+v, want kind api status 403", env.Error)
	}
	if !strings.Contains(env.Error.Message, "You do not have access") {
		t.Errorf("message = %q, want provider error text", env.Error.Message)
	}
}

func TestErrorMap422RendersFieldMap(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/people": {status: http.StatusUnprocessableEntity, body: `{"errors":{"email_address":["is invalid"]}}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "create", "--email", "bad")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "email_address") || !strings.Contains(stderr, "is invalid") {
		t.Errorf("stderr = %q, want the 422 field map rendered", stderr)
	}
}

func TestCredentialRejectedOn401(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/me": {status: http.StatusUnauthorized, body: `{"error":"Unauthorized"}`},
	})
	defer srv.Close()

	result, _, _ := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("expected CredentialRejected on 401")
	}
}

func TestRateLimitHeaderSurfaced(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/people": {
			status:  http.StatusTooManyRequests,
			body:    `{"error":"Throttled"}`,
			headers: map[string]string{"x-ratelimit-remaining-minute": "0"},
		},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "remaining-minute") && !strings.Contains(stderr, "rate limit") {
		t.Errorf("stderr = %q, want rate-limit context", stderr)
	}
}
