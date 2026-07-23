package intercom

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingToken_Exit1(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"admin", "me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "INTERCOM_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestMissingToken_JSONEnvelope(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	_, _ = svc.Execute(context.Background(), []string{"--json", "admin", "me"}, map[string]string{})
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestHeadersAndAuth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"admin","id":"42"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "admin", "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Version != intercomVersion {
		t.Errorf("Intercom-Version = %q, want %s", got.Version, intercomVersion)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"id":"42"`) {
		t.Errorf("stdout = %q, want verbatim passthrough", stdout)
	}
}

func TestVerbatimPassthroughNewline(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"type":"admin.list","admins":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "admin", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if stdout != `{"type":"admin.list","admins":[]}`+"\n" {
		t.Errorf("stdout = %q, want verbatim body + newline", stdout)
	}
}

func TestAPIError_ExitAndMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity,
		`{"type":"error.list","errors":[{"code":"parameter_invalid","message":"bad admin_id"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "admin", "get", "--id", "x")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (api error)", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("422 should not mark the credential rejected")
	}
	if !strings.Contains(stderr, "parameter_invalid") || !strings.Contains(stderr, "bad admin_id") {
		t.Errorf("stderr = %q, want code+message from error.list", stderr)
	}
	if !strings.Contains(stderr, "HTTP 422") {
		t.Errorf("stderr = %q, want HTTP status", stderr)
	}
}

func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized,
		`{"type":"error.list","errors":[{"code":"unauthorized","message":"Access Token Invalid"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "admin", "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
	if !strings.Contains(stderr, "unauthorized") {
		t.Errorf("stderr = %q, want unauthorized message", stderr)
	}
}

func TestAPIError_JSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound,
		`{"type":"error.list","errors":[{"code":"not_found","message":"nope"}]}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "--json", "admin", "get", "--id", "x")
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", env.Error.Status)
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "conversation", "frobnicate")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
}

func TestMissingRequiredFlag_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "conversation", "get")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 (missing --id)", result.ExitCode)
	}
	if got.Method != "" {
		t.Error("no HTTP request should be made when a required flag is missing")
	}
}

func TestBareGroup_ShowsHelpExit0(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "conversation")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (bare group shows help)", result.ExitCode)
	}
}

func TestBuildSearchBody(t *testing.T) {
	t.Run("raw query only", func(t *testing.T) {
		body, err := buildSearchBody(searchFlags{query: `{"field":"state","operator":"=","value":"open"}`}, nil)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		q, ok := body["query"].(map[string]any)
		if !ok || q["field"] != "state" {
			t.Errorf("query = %v, want decoded object", body["query"])
		}
	})
	t.Run("single filter is not AND-wrapped", func(t *testing.T) {
		body, err := buildSearchBody(searchFlags{}, []map[string]any{filterEq("email", "a@b.com")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		q := body["query"].(map[string]any)
		if q["operator"] != "=" || q["field"] != "email" {
			t.Errorf("query = %v, want single equality filter", q)
		}
	})
	t.Run("multiple filters AND-wrapped", func(t *testing.T) {
		body, err := buildSearchBody(searchFlags{}, []map[string]any{
			filterEq("state", "open"), filterGT("updated_at", "1"),
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		q := body["query"].(map[string]any)
		if q["operator"] != "AND" {
			t.Errorf("operator = %v, want AND", q["operator"])
		}
		if vals, ok := q["value"].([]map[string]any); !ok || len(vals) != 2 {
			t.Errorf("value = %v, want 2 filters", q["value"])
		}
	})
	t.Run("raw query and filters are mutually exclusive", func(t *testing.T) {
		_, err := buildSearchBody(searchFlags{query: `{}`}, []map[string]any{filterEq("email", "x")})
		if err == nil {
			t.Fatal("want usage error for --query + convenience filters")
		}
	})
	t.Run("no query source is an error", func(t *testing.T) {
		if _, err := buildSearchBody(searchFlags{}, nil); err == nil {
			t.Fatal("want usage error when neither query nor filters supplied")
		}
	})
	t.Run("pagination attached", func(t *testing.T) {
		body, err := buildSearchBody(searchFlags{perPage: 30, startingAfter: "cur"}, []map[string]any{filterEq("email", "x")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		p := body["pagination"].(map[string]any)
		if p["per_page"] != 30 || p["starting_after"] != "cur" {
			t.Errorf("pagination = %v", p)
		}
	})
}
