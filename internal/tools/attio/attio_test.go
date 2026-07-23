package attio

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

func TestWhoamiSummaryAndBearerInjection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self": {status: http.StatusOK, body: `{"workspace_id":"ws-1","workspace_name":"Acme","workspace_slug":"acme","authorized_by_workspace_member_id":"m-1"}`},
	})
	defer srv.Close()

	out, errStr, exit := runService(t, srv, "whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	if !strings.Contains(out, "ws-1") || !strings.Contains(out, "Acme") || !strings.Contains(out, "acme") {
		t.Errorf("whoami summary missing fields: %q", out)
	}
	req := findReq(reqs, http.MethodGet, "/v2/self")
	if req == nil {
		t.Fatal("no GET /v2/self recorded")
	}
	if req.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", req.Auth)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
}

func TestWhoamiJSONEmitsVerbatim(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self": {status: http.StatusOK, body: `{"workspace_id":"ws-1","workspace_name":"Acme"}`},
	})
	defer srv.Close()

	out, _, exit := runService(t, srv, "whoami", "--json")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("--json output is not valid JSON: %v (%s)", err, out)
	}
	if m["workspace_id"] != "ws-1" {
		t.Errorf("workspace_id = %v, want ws-1", m["workspace_id"])
	}
}

func TestMissingTokenFailsFastExit1(t *testing.T) {
	var out, errBuf strings.Builder
	s := &Service{Out: &out, Err: &errBuf}
	res, err := s.Execute(context.Background(), []string{"whoami"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "ATTIO_ACCESS_TOKEN") {
		t.Errorf("stderr = %q, want mention of ATTIO_ACCESS_TOKEN", errBuf.String())
	}
}

func TestMissingTokenJSONErrorEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	s := &Service{Out: &out, Err: &errBuf}
	_, err := s.Execute(context.Background(), []string{"whoami", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestAPIErrorExit1AndJSONEnvelopeCarriesStatus(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/objects/nope": {status: http.StatusNotFound, body: `{"status_code":404,"type":"invalid_request_error","code":"not_found","message":"Object not found."}`},
	})
	defer srv.Close()

	out, errStr, exit := runService(t, srv, "object", "get", "nope", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if out != "" {
		t.Errorf("stdout should be empty on error, got %q", out)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errStr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errStr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Errorf("envelope = %+v, want kind=api status=404", env.Error)
	}
}

func TestUnauthorizedClassifiedAsCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self": {status: http.StatusUnauthorized, body: `{"status_code":401,"type":"authentication_error","code":"invalid_token","message":"Bad token."}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &strings.Builder{}, Err: &strings.Builder{}}
	res, err := s.Execute(context.Background(), []string{"whoami"}, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 was not classified as a credential rejection")
	}
}

func TestForbiddenIsNotCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/self": {status: http.StatusForbidden, body: `{"status_code":403,"type":"authorization_error","code":"forbidden","message":"Missing scope."}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &strings.Builder{}, Err: &strings.Builder{}}
	res, err := s.Execute(context.Background(), []string{"whoami"}, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("403 (scope) must not be a credential rejection")
	}
}

func TestUnknownSubcommandIsUsageExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, exit := runService(t, srv, "record", "frobnicate")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", exit)
	}
	if len(reqs) != 0 {
		t.Errorf("unknown subcommand must not reach the API, got %d requests", len(reqs))
	}
}

// TestExecutionResultContractSanity guards the exit-code helper wiring the
// service relies on, so a refactor of execution.Failure cannot silently break
// attio's classification.
func TestExecutionResultContractSanity(t *testing.T) {
	if execution.Failure(&apiError{msg: "x", status: 500}).ExitCode != 1 {
		t.Fatal("apiError must map to exit 1")
	}
}
