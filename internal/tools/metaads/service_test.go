package metaads

import (
	"net/http"
	"strings"
	"testing"
)

func TestExecuteRequiresAccessToken(t *testing.T) {
	code, _, stderr := run(t, nil, map[string]string{}, "accounts", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "META_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr)
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "not-a-command")
	if code == 0 {
		t.Fatal("unknown subcommand returned exit code 0")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestJSONFlagHelpDescribesJSONL(t *testing.T) {
	code, stdout, stderr := run(t, nil, fullEnv(), "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "single-result JSON; multi-result commands may emit JSONL") {
		t.Fatalf("help = %q", stdout)
	}
}

func TestGraphVersionAndBearerInjection(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "accounts", "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(captured.Path, "/"+GraphVersion+"/") {
		t.Fatalf("path = %q, want %s prefix", captured.Path, GraphVersion)
	}
	if captured.Auth != "Bearer meta-user-token" {
		t.Fatalf("auth = %q, want Bearer header", captured.Auth)
	}
	if strings.Contains(captured.RawQuery, "access_token") {
		t.Fatalf("token leaked into query: %q", captured.RawQuery)
	}
}
