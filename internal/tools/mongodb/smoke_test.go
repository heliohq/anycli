package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// realMongosh returns the PATH-resolved mongosh or skips the test. Smoke
// tests never download: they only run when a real mongosh is already
// installed on the machine.
func realMongosh(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("mongosh")
	if err != nil {
		t.Skipf("mongosh not found in PATH; skipping smoke test: %v", err)
	}
	return path
}

// TestSmokeMongoshEvalJSONOutput drives the real mongosh with the wrapper's
// fixed flag set (minus the connect prelude — no server is required) and
// asserts --eval= evaluation, --json output parsing, the exit code, and that
// the process completes without a TTY.
func TestSmokeMongoshEvalJSONOutput(t *testing.T) {
	bin := realMongosh(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	scopedHome := t.TempDir()
	args := append(append([]string{}, fixedMongoshFlags...), "--eval=({sum: 1 + 1})")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = childEnv("mongodb://unused.invalid/", scopedHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("mongosh failed: %v\nstderr: %s", err, stderr.String())
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if sum, ok := result["sum"].(float64); !ok || sum != 2 {
		t.Errorf("result = %v, want sum 2", result)
	}
}

// TestSmokeMongoshShellFlagAsScriptDoesNotOpenShell proves the injection
// contract against the real binary: "--shell" delivered as a fused --eval=
// payload is evaluated as JavaScript (and fails), it does not become a
// mongosh flag or an interactive REPL — the process terminates on its own.
func TestSmokeMongoshShellFlagAsScriptDoesNotOpenShell(t *testing.T) {
	bin := realMongosh(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := append(append([]string{}, fixedMongoshFlags...), "--eval=--shell")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = childEnv("mongodb://unused.invalid/", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // expected to fail; the assertions below carry the test
	if ctx.Err() != nil {
		t.Fatal("mongosh hung — --eval=--shell must not open an interactive shell")
	}
	if code := cmd.ProcessState.ExitCode(); code == 0 {
		t.Errorf("exit = 0, want non-zero for an invalid script\nstdout: %s", stdout.String())
	}
}

// TestSmokeServiceEvalUnreachableHost runs the full service pipeline (connect
// prelude + env injection + redaction + classification) against the real
// mongosh with an unreachable host: the invocation must fail fast, must NOT
// reject the credential (network != auth), and must not leak the DSN.
func TestSmokeServiceEvalUnreachableHost(t *testing.T) {
	realMongosh(t)
	// Keep resolution hermetic: an empty pin root forces the PATH level.
	t.Setenv("HELIO_BIN_DIR", t.TempDir())

	dsn := "mongodb://smokeuser:smokepw@127.0.0.1:1/?connectTimeoutMS=2000&serverSelectionTimeoutMS=2000&directConnection=true"
	var out, errOut bytes.Buffer
	svc := &Service{Out: &out, Err: &errOut}

	res, err := svc.Execute(context.Background(), []string{"eval", "db.stats()", "--timeout", "60s"},
		map[string]string{EnvConnectionString: dsn})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("exit = 0, want failure for an unreachable host\nstdout: %s", out.String())
	}
	if res.CredentialRejected {
		t.Error("network failure must not reject the credential")
	}
	combined := out.String() + errOut.String()
	if strings.Contains(combined, "smokepw") || strings.Contains(combined, dsn) {
		t.Errorf("output leaked the DSN or password:\n%s", combined)
	}
}
