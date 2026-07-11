package x

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExecuteRequiresAccessToken(t *testing.T) {
	var stderr bytes.Buffer
	svc := &Service{Err: &stderr}
	code, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "X_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr.String())
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "not-a-command")
	if code == 0 {
		t.Fatal("unknown subcommand returned exit code 0")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestJSONFlagHelpDescribesJSONL(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, stdout, stderr := run(t, server, fullEnv(), "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "single-result JSON; multi-result commands may emit JSONL") {
		t.Fatalf("help = %q", stdout)
	}
}
