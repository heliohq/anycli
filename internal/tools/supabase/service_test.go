package supabase

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

const testAccessToken = "sbp_test-secret-access-token"

// fakeRun records the official CLI subprocess contract without performing
// network or filesystem work.
type fakeRun struct {
	exitCode int
	stdout   string
	stderr   string
	err      error

	called bool
	args   []string
	env    []string
}

// run implements Runner for command and credential-boundary tests.
func (f *fakeRun) run(_ context.Context, args, env []string) (int, []byte, []byte, error) {
	f.called = true
	f.args = slices.Clone(args)
	f.env = slices.Clone(env)
	return f.exitCode, []byte(f.stdout), []byte(f.stderr), f.err
}

// execute runs one service invocation with captured streams.
func execute(t *testing.T, fake *fakeRun, token string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	service := &Service{Run: fake.run, Out: &stdout, Err: &stderr}
	result, err := service.Execute(context.Background(), args, map[string]string{EnvAccessToken: token})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	return result, stdout.String(), stderr.String()
}

// envValue finds an exact child environment binding and its occurrence count.
func envValue(env []string, key string) (string, int) {
	value, count := "", 0
	for _, binding := range env {
		name, candidate, ok := strings.Cut(binding, "=")
		if ok && name == key {
			value = candidate
			count++
		}
	}
	return value, count
}

// TestExecuteProjectsList pins JSON output, non-interactive execution, secret
// redaction, and the isolated Supabase CLI home/environment boundary.
func TestExecuteProjectsList(t *testing.T) {
	t.Setenv("SUPABASE_ACCESS_TOKEN", "ambient-token")
	t.Setenv("SUPABASE_DB_PASSWORD", "ambient-password")
	t.Setenv("SUPABASE_PROJECT_ID", "ambient-project")
	fake := &fakeRun{stdout: `{"token":"` + testAccessToken + `"}`}

	result, stdout, _ := execute(t, fake, testAccessToken, "projects", "list")
	if result.ExitCode != 0 || result.CredentialRejected {
		t.Fatalf("result = %+v, want success", result)
	}
	wantArgs := []string{
		"projects", "list",
		"--output", "json",
		"--output-format", "json",
		"--agent", "yes",
		"--yes",
	}
	if !slices.Equal(fake.args, wantArgs) {
		t.Errorf("argv = %#v, want %#v", fake.args, wantArgs)
	}
	if strings.Contains(stdout, testAccessToken) || !strings.Contains(stdout, "[REDACTED]") {
		t.Errorf("stdout = %q, want access token redacted", stdout)
	}
	if got, count := envValue(fake.env, EnvAccessToken); got != testAccessToken || count != 1 {
		t.Errorf("%s = %q (%d bindings), want injected token exactly once", EnvAccessToken, got, count)
	}
	if got, count := envValue(fake.env, "SUPABASE_HOME"); got == "" || count != 1 {
		t.Errorf("SUPABASE_HOME = %q (%d bindings), want one isolated directory", got, count)
	}
	for _, key := range []string{"SUPABASE_DB_PASSWORD", "SUPABASE_PROJECT_ID"} {
		if value, count := envValue(fake.env, key); value != "" || count != 0 {
			t.Errorf("child env leaked %s=%q (%d bindings)", key, value, count)
		}
	}
	for key, want := range map[string]string{
		"SUPABASE_TELEMETRY_DISABLED": "1",
		"SUPABASE_NO_KEYRING":         "1",
		"SUPABASE_NO_UPDATE_NOTIFIER": "1",
		"DO_NOT_TRACK":                "1",
	} {
		if got, count := envValue(fake.env, key); got != want || count != 1 {
			t.Errorf("%s = %q (%d bindings), want %q exactly once", key, got, count, want)
		}
	}
}

// TestMissingAccessTokenFailsBeforeSpawn keeps credential lookup failures out
// of the subprocess and distinct from rejected provider credentials.
func TestMissingAccessTokenFailsBeforeSpawn(t *testing.T) {
	fake := &fakeRun{}
	result, _, stderr := execute(t, fake, "", "projects", "list")
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 without credential rejection", result)
	}
	if fake.called {
		t.Error("runner called without an access token")
	}
	if !strings.Contains(stderr, EnvAccessToken+" is not set") {
		t.Errorf("stderr = %q, want missing-token message", stderr)
	}
}

// TestUsageErrorsExitTwo verifies that AnyCLI rejects unexposed official CLI
// flags before spawning the binary.
func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"functions", "deploy", "hello", "--project-ref", "abcdefghijklmnopqrst", "--local"},
		{"gen", "types", "--local"},
		{"branches", "get", "preview", "--project-ref", "abcdefghijklmnopqrst"},
		{"start"},
	} {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken, args...)
		if result.ExitCode != 2 {
			t.Errorf("args %v: result = %+v, want usage exit 2", args, result)
		}
		if fake.called {
			t.Errorf("args %v: runner called for rejected command", args)
		}
	}
}

// TestFunctionDeployForcesRemoteAPIBundling proves the wrapper cannot fall
// back to Docker while preserving the exposed remote deployment options.
func TestFunctionDeployForcesRemoteAPIBundling(t *testing.T) {
	fake := &fakeRun{}
	result, _, _ := execute(t, fake, testAccessToken,
		"functions", "deploy", "hello", "goodbye",
		"--project-ref", "abcdefghijklmnopqrst",
		"--workdir", "/workspace/app",
		"--prune",
		"--jobs", "3",
	)
	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want success", result)
	}
	for _, want := range []string{
		"functions", "deploy", "hello", "goodbye",
		"--project-ref=abcdefghijklmnopqrst",
		"--workdir=/workspace/app",
		"--prune=true",
		"--jobs=3",
		"--use-api",
	} {
		if !slices.Contains(fake.args, want) {
			t.Errorf("argv = %#v, missing %q", fake.args, want)
		}
	}
	for _, forbidden := range []string{"--local", "--linked", "--db-url"} {
		if slices.Contains(fake.args, forbidden) {
			t.Errorf("argv = %#v, contains forbidden %q", fake.args, forbidden)
		}
	}
}

// TestGenTypesRequiresProjectID keeps generation on the token-authenticated
// Management API path instead of local, linked, or direct-database modes.
func TestGenTypesRequiresProjectID(t *testing.T) {
	fake := &fakeRun{}
	result, _, _ := execute(t, fake, testAccessToken,
		"gen", "types",
		"--project-id", "abcdefghijklmnopqrst",
		"--lang", "go",
		"--schema", "public,auth",
	)
	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want success", result)
	}
	for _, want := range []string{
		"--project-id=abcdefghijklmnopqrst",
		"--lang=go",
		"--schema=public,auth",
	} {
		if !slices.Contains(fake.args, want) {
			t.Errorf("argv = %#v, missing %q", fake.args, want)
		}
	}

	missing := &fakeRun{}
	missingResult, _, _ := execute(t, missing, testAccessToken, "gen", "types")
	if missingResult.ExitCode != 2 || missing.called {
		t.Errorf("missing project id: result=%+v called=%v, want usage exit before spawn", missingResult, missing.called)
	}
}

// TestDBQueryForwardsOfficialInputs pins the wrapper boundary for SQL input,
// target selection, and project-relative file resolution without running SQL.
func TestDBQueryForwardsOfficialInputs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantArgs []string
	}{
		{
			name: "linked positional query",
			args: []string{
				"db", "query", "select id from public.accounts",
				"--linked", "--project-ref", "abcdefghijklmnopqrst",
			},
			wantArgs: []string{
				"db", "query", "select id from public.accounts",
				"--linked=true", "--project-ref=abcdefghijklmnopqrst",
				"--output", "json", "--output-format", "json", "--agent", "yes", "--yes",
			},
		},
		{
			name: "direct database file",
			args: []string{
				"db", "query", "--db-url", "postgresql://user:password@example.test:5432/postgres",
				"--file", "queries/report.sql", "--workdir", "/workspace/app",
			},
			wantArgs: []string{
				"db", "query",
				"--db-url=postgresql://user:password@example.test:5432/postgres",
				"--file=queries/report.sql", "--workdir=/workspace/app",
				"--output", "json", "--output-format", "json", "--agent", "yes", "--yes",
			},
		},
		{
			name: "explicit local target",
			args: []string{"db", "query", "select 1", "--local"},
			wantArgs: []string{
				"db", "query", "select 1", "--local=true",
				"--output", "json", "--output-format", "json", "--agent", "yes", "--yes",
			},
		},
		{
			name: "official default target",
			args: []string{"db", "query", "select 1"},
			wantArgs: []string{
				"db", "query", "select 1",
				"--output", "json", "--output-format", "json", "--agent", "yes", "--yes",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeRun{}
			result, _, _ := execute(t, fake, testAccessToken, test.args...)
			if result.ExitCode != 0 {
				t.Fatalf("result = %+v, want success", result)
			}
			if !slices.Equal(fake.args, test.wantArgs) {
				t.Errorf("argv = %#v, want %#v", fake.args, test.wantArgs)
			}
		})
	}
}

// TestStorageDownloadOnlyAllowsRemoteToLocal prevents the official cp command
// from becoming an upload or remote-to-remote mutation through positional
// arguments.
func TestStorageDownloadOnlyAllowsRemoteToLocal(t *testing.T) {
	fake := &fakeRun{}
	result, _, _ := execute(t, fake, testAccessToken,
		"storage", "download", "ss:///assets/docs", "./docs",
		"--project-ref", "abcdefghijklmnopqrst",
		"--recursive",
	)
	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want success", result)
	}
	wantPrefix := []string{"storage", "cp", "ss:///assets/docs", "./docs"}
	if len(fake.args) < len(wantPrefix) || !slices.Equal(fake.args[:len(wantPrefix)], wantPrefix) {
		t.Errorf("argv prefix = %#v, want %#v", fake.args, wantPrefix)
	}

	for _, args := range [][]string{
		{"storage", "download", "./local", "ss:///assets/file", "--project-ref", "abcdefghijklmnopqrst"},
		{"storage", "download", "ss:///assets/file", "ss:///other/file", "--project-ref", "abcdefghijklmnopqrst"},
	} {
		rejected := &fakeRun{}
		rejectedResult, _, _ := execute(t, rejected, testAccessToken, args...)
		if rejectedResult.ExitCode != 2 || rejected.called {
			t.Errorf("args %v: result=%+v called=%v, want usage exit before spawn", args, rejectedResult, rejected.called)
		}
	}
}

// TestCredentialRejectionClassification lets an embedding host refresh a
// rejected OAuth token while leaving permission failures alone.
func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name       string
		stdout     string
		wantReject bool
	}{
		{
			name:       "invalid access token",
			stdout:     `{"_tag":"Error","error":{"code":"LegacyInvalidAccessTokenError","message":"Invalid access token"}}`,
			wantReject: true,
		},
		{
			name:       "unauthorized API response",
			stdout:     `{"_tag":"Error","error":{"code":"LegacyProjectsListUnexpectedStatusError","message":"Unexpected response: {\"message\":\"Unauthorized\"}"}}`,
			wantReject: true,
		},
		{
			name:       "insufficient scope",
			stdout:     `{"_tag":"Error","error":{"code":"LegacyUnexpectedStatusError","message":"Forbidden action"}}`,
			wantReject: false,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeRun{exitCode: 1, stdout: test.stdout}
			result, _, _ := execute(t, fake, testAccessToken, "projects", "list")
			if result.ExitCode != 1 || result.CredentialRejected != test.wantReject {
				t.Errorf("result = %+v, want exit 1 reject=%v", result, test.wantReject)
			}
		})
	}
}

// TestRunnerFailuresRemainRuntimeErrors ensures process-launch failures are
// surfaced as command failures rather than engine errors or usage mistakes.
func TestRunnerFailuresRemainRuntimeErrors(t *testing.T) {
	fake := &fakeRun{err: errors.New("exec failed")}
	result, _, stderr := execute(t, fake, testAccessToken, "projects", "list")
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Errorf("result = %+v, want runtime exit 1", result)
	}
	if !strings.Contains(stderr, "exec failed") {
		t.Errorf("stderr = %q, want runner failure", stderr)
	}
}

// TestCommandSurfaceAndSideEffects pins the complete public face. Any command
// not listed here is intentionally unavailable in this token-only wrapper.
func TestCommandSurfaceAndSideEffects(t *testing.T) {
	want := map[string]string{
		"backups list":                "false",
		"branches create":             "true",
		"branches delete":             "true",
		"branches list":               "false",
		"branches pause":              "true",
		"branches unpause":            "true",
		"branches update":             "true",
		"db query":                    "true",
		"domains get":                 "false",
		"functions delete":            "true",
		"functions deploy":            "true",
		"functions download":          "false",
		"functions list":              "false",
		"gen types":                   "false",
		"network-bans get":            "false",
		"network-bans remove":         "true",
		"network-restrictions get":    "false",
		"network-restrictions update": "true",
		"orgs list":                   "false",
		"postgres-config delete":      "true",
		"postgres-config get":         "false",
		"postgres-config update":      "true",
		"projects api-keys":           "false",
		"projects delete":             "true",
		"projects list":               "false",
		"secrets list":                "false",
		"snippets download":           "false",
		"snippets list":               "false",
		"ssl-enforcement get":         "false",
		"ssl-enforcement update":      "true",
		"sso info":                    "false",
		"sso list":                    "false",
		"sso show":                    "false",
		"storage download":            "false",
		"storage ls":                  "false",
		"vanity-subdomains get":       "false",
	}

	root := (&Service{}).NewCommandTree()
	got := map[string]string{}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if !command.HasSubCommands() && (command.RunE != nil || command.Run != nil) {
			relative := strings.TrimPrefix(command.CommandPath(), root.CommandPath()+" ")
			got[relative] = command.Annotations["anycli.side_effect"]
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	if len(got) != len(want) {
		t.Errorf("runnable command count = %d, want %d\ngot: %#v", len(got), len(want), got)
	}
	for path, sideEffect := range want {
		if got[path] != sideEffect {
			t.Errorf("%s side effect = %q, want %q", path, got[path], sideEffect)
		}
	}
}
