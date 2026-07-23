package square

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAuthAndVersionHeadersInjected proves every call carries Bearer auth and
// the fixed Square-Version header, and that the provider JSON is emitted
// verbatim on stdout with a trailing newline.
func TestAuthAndVersionHeadersInjected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"locations":[{"id":"L1"}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "location", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", got.Auth, "Bearer tok-123")
	}
	if got.SquareVersion != squareVersion {
		t.Errorf("Square-Version = %q, want %q", got.SquareVersion, squareVersion)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/locations" {
		t.Errorf("got %s %s, want GET /v2/locations", got.Method, got.Path)
	}
	if stdout != `{"locations":[{"id":"L1"}]}`+"\n" {
		t.Errorf("stdout = %q, want verbatim provider body + newline", stdout)
	}
}

// TestVerbatimPassthroughPreservesCursor confirms a paginated envelope with a
// cursor reaches the agent untouched (no re-wrapping).
func TestVerbatimPassthroughPreservesCursor(t *testing.T) {
	body := `{"payments":[{"id":"P1"}],"cursor":"CURSOR123"}`
	var got capturedRequest
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "payment", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if strings.TrimSpace(stdout) != body {
		t.Errorf("stdout = %q, want verbatim %q", stdout, body)
	}
}

// TestAPIErrorSurfacesDetailExit1 proves a non-2xx with Square's errors[] shape
// renders errors[].detail and exits 1.
func TestAPIErrorSurfacesDetailExit1(t *testing.T) {
	errBody := `{"errors":[{"category":"INVALID_REQUEST_ERROR","code":"NOT_FOUND","detail":"Location not found."}]}`
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, errBody, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "location", "get", "--location-id", "nope")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "Location not found.") {
		t.Errorf("stderr = %q, want it to carry errors[].detail", stderr)
	}
	if !strings.Contains(stderr, "404") {
		t.Errorf("stderr = %q, want it to carry the HTTP status", stderr)
	}
}

// TestJSONErrorEnvelope proves --json renders the structured
// {"error":{"kind":"api","status":…}} envelope on stderr.
func TestJSONErrorEnvelope(t *testing.T) {
	errBody := `{"errors":[{"category":"AUTHENTICATION_ERROR","code":"UNAUTHORIZED","detail":"bad token"}]}`
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, errBody, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "location", "list", "--json")
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
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", env.Error.Status)
	}
}

// TestUsageErrorExit2 proves a bad --body JSON is a usage error (exit 2).
func TestUsageErrorExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "customer", "search", "--body", "{not json")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", exit, stderr)
	}
	if got.Method != "" {
		t.Errorf("server was called (%s %s) but the request should never leave on a parse error", got.Method, got.Path)
	}
}

// TestUnknownSubcommandExit2 proves an unknown subcommand is a usage error.
func TestUnknownSubcommandExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "payment", "bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
}

// TestMissingTokenExit1 proves the pre-parse missing-token guard exits 1 and,
// under --json, renders a usage-kind envelope.
func TestMissingTokenExit1(t *testing.T) {
	result, _, stderr := runNoToken(t, "location", "list", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, `"kind":"usage"`) {
		t.Errorf("stderr = %q, want a usage-kind JSON envelope", stderr)
	}
}

// TestBaseURLFromEnv proves SQUARE_BASE_URL overrides the host when BaseURL is
// unset on the struct (the L2 sandbox path).
func TestBaseURLFromEnv(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"locations":[]}`, &got)
	defer srv.Close()

	svc := &Service{HC: srv.Client()}
	// Route through Execute with the env override; capture via server.
	_, err := svc.Execute(t.Context(), []string{"location", "list"}, map[string]string{
		EnvAccessToken: "tok-xyz",
		EnvBaseURL:     srv.URL,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got.Path != "/v2/locations" {
		t.Errorf("request did not reach the env-configured base URL (path=%q)", got.Path)
	}
	if got.Auth != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, want Bearer tok-xyz", got.Auth)
	}
}
