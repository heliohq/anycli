package posthog

import (
	"net/http"
	"strings"
	"testing"
)

func TestMissingTokenFailsBeforeAnyRequest(t *testing.T) {
	svc := &Service{BaseURL: "https://unused.example"}
	exit, stdout, stderr := runService(t, svc, map[string]string{}, "whoami")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "POSTHOG_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUnknownSubcommandIsUsageErrorExit2(t *testing.T) {
	svc := &Service{BaseURL: "https://unused.example"}
	exit, _, _ := runService(t, svc, map[string]string{EnvAccessToken: testToken}, "nonsense")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}

func TestUsageErrorJSONEnvelope(t *testing.T) {
	svc := &Service{BaseURL: "https://unused.example"}
	// query run with neither --hogql nor --query-json is a usage error.
	exit, _, stderr := runService(t, svc, map[string]string{EnvAccessToken: testToken},
		"query", "run", "--project", "1", "--json")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	envelope := decodeErrorEnvelope(t, stderr)
	if envelope["kind"] != "usage" {
		t.Fatalf("kind = %v, want usage", envelope["kind"])
	}
	if _, hasStatus := envelope["status"]; hasStatus {
		t.Fatalf("usage error must not carry an HTTP status: %v", envelope)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	srv := singleRouteServer(t, http.StatusNotFound, `{"type":"invalid_request","code":"not_found","detail":"Not found."}`, &capturedRequest{})
	defer srv.Close()
	exit, _, stderr := run(t, srv, "project", "list", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	envelope := decodeErrorEnvelope(t, stderr)
	if envelope["kind"] != "api" {
		t.Fatalf("kind = %v, want api", envelope["kind"])
	}
	if envelope["status"] != float64(http.StatusNotFound) {
		t.Fatalf("status = %v, want 404", envelope["status"])
	}
	// The PostHog detail must surface verbatim.
	if !strings.Contains(envelope["message"].(string), "Not found.") {
		t.Fatalf("message = %v, want PostHog detail passthrough", envelope["message"])
	}
}

func TestWhoamiEmitsBodyAndRegionNote(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"email":"ada@example.com","uuid":"u-1"}`, &got)
	defer srv.Close()
	exit, stdout, stderr := run(t, srv, "whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Path != "/api/users/@me" || got.Method != http.MethodGet {
		t.Fatalf("request = %s %s, want GET /api/users/@me", got.Method, got.Path)
	}
	if got.Auth != "Bearer "+testToken {
		t.Fatalf("Authorization = %q", got.Auth)
	}
	if !strings.Contains(stdout, "ada@example.com") {
		t.Fatalf("stdout = %q, want user body", stdout)
	}
	if !strings.Contains(stderr, "resolved region host:") {
		t.Fatalf("stderr = %q, want region note", stderr)
	}
}
