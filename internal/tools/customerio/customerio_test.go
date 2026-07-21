package customerio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"workspace", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey+" is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

func TestExecute_MissingKey_JSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"--json", "workspace", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if uErr := json.Unmarshal([]byte(errBuf.String()), &env); uErr != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", uErr, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestBearerAuthAndAccept(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"workspaces":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "workspace", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Auth != "Bearer key-123" {
		t.Errorf("Authorization = %q, want %q", got.Auth, "Bearer key-123")
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.Path != "/v1/workspaces" {
		t.Errorf("path = %q, want /v1/workspaces", got.Path)
	}
}

func TestRegionEU_SwitchesBaseURL(t *testing.T) {
	// With no BaseURL override, --region eu must resolve the EU host. We can't
	// reach the real host in a unit test, so assert the resolver directly.
	svc := &Service{}
	root := svc.newRoot("k")
	root.SetArgs([]string{"--region", "eu", "workspace", "list"})
	// Resolve region from a child command's flag set via the persistent flag.
	if err := root.PersistentFlags().Set("region", "eu"); err != nil {
		t.Fatalf("set region: %v", err)
	}
	base, err := svc.regionBase(root)
	if err != nil {
		t.Fatalf("regionBase: %v", err)
	}
	if base != EUBaseURL {
		t.Errorf("EU base = %q, want %q", base, EUBaseURL)
	}
}

func TestRegionInvalid_UsageError(t *testing.T) {
	svc := &Service{}
	root := svc.newRoot("k")
	if err := root.PersistentFlags().Set("region", "apac"); err != nil {
		t.Fatalf("set region: %v", err)
	}
	if _, err := svc.regionBase(root); err == nil {
		t.Fatal("expected a usage error for an invalid region")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"meta":{"error":"nope"}}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "workspace", "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "nope") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

func TestAPIError_JSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"meta":{"error":"missing"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "campaign", "get", "--id", "9")
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
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusNotFound {
		t.Errorf("envelope = %+v, want kind=api status=404", env.Error)
	}
}

func TestUnknownSubcommand_ExitsUsage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "person", "frobnicate")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for an unknown subcommand", exit)
	}
}

func TestProviderJSONPassthrough(t *testing.T) {
	var got capturedRequest
	body := `{"workspaces":[{"id":7,"name":"Prod"}]}`
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "workspace", "list")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if strings.TrimSpace(stdout) != body {
		t.Errorf("stdout = %q, want verbatim provider JSON %q", stdout, body)
	}
}
