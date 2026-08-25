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

// TestExecuteProjectsList pins JSON output, credential isolation, secret
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

// TestUnknownCommandsExitTwo keeps the capability envelope at the command
// boundary while leaving official flag validation to Supabase.
func TestUnknownCommandsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"--experimental", "projects", "list"},
		{"branches", "get", "preview", "--project-ref", "abcdefghijklmnopqrst"},
		{"projects", "api-keys", "--project-ref", "abcdefghijklmnopqrst"},
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

// TestAllowedCommandPassesOfficialFlagsVerbatim catches wrappers that reject,
// normalize, reorder, or otherwise reinterpret flags owned by Supabase CLI.
func TestAllowedCommandPassesOfficialFlagsVerbatim(t *testing.T) {
	fake := &fakeRun{}
	result, _, _ := execute(t, fake, testAccessToken,
		"projects", "list",
		"--future-filter", "active",
		"--future-bool",
		"--experimental",
		"--timeout", "30s",
	)
	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want success", result)
	}
	wantArgs := []string{
		"projects", "list",
		"--future-filter", "active",
		"--future-bool",
		"--experimental",
		"--timeout", "30s",
		"--output", "json",
		"--output-format", "json",
		"--agent", "yes",
	}
	if !slices.Equal(fake.args, wantArgs) {
		t.Errorf("argv = %#v, want verbatim official args %#v", fake.args, wantArgs)
	}
}

// TestProviderFlagValidationIsDelegated proves provider-owned flag
// combinations reach the official CLI, which remains their source of truth.
func TestProviderFlagValidationIsDelegated(t *testing.T) {
	t.Run("official error envelope preserves provider exit", func(t *testing.T) {
		fake := &fakeRun{
			exitCode: 1,
			stderr:   `{"_tag":"Error","error":{"code":"UnrecognizedOption","message":"Unknown option --future"}}`,
		}
		result, _, _ := execute(t, fake, testAccessToken, "projects", "list", "--future")
		if result.ExitCode != 1 {
			t.Errorf("result = %+v, want official CLI exit 1", result)
		}
	})

	t.Run("conflicting database targets", func(t *testing.T) {
		fake := &fakeRun{exitCode: 1, stderr: "official validation error"}
		result, _, _ := execute(t, fake, testAccessToken,
			"db", "query", "select 1", "--linked", "--local",
		)
		if result.ExitCode != 1 {
			t.Errorf("result = %+v, want official CLI exit 1", result)
		}
		if !fake.called {
			t.Fatal("official CLI was not called")
		}
		for _, want := range []string{"--linked", "--local"} {
			if !slices.Contains(fake.args, want) {
				t.Errorf("argv = %#v, missing %q", fake.args, want)
			}
		}
	})

	t.Run("conflicting SSL choices", func(t *testing.T) {
		fake := &fakeRun{exitCode: 1, stderr: "official validation error"}
		result, _, _ := execute(t, fake, testAccessToken,
			"ssl-enforcement", "update",
			"--project-ref", "abcdefghijklmnopqrst",
			"--enable-db-ssl-enforcement", "--disable-db-ssl-enforcement",
		)
		if result.ExitCode != 1 {
			t.Errorf("result = %+v, want official CLI exit 1", result)
		}
		if !fake.called {
			t.Fatal("official CLI was not called")
		}
		for _, want := range []string{"--enable-db-ssl-enforcement", "--disable-db-ssl-enforcement"} {
			if !slices.Contains(fake.args, want) {
				t.Errorf("argv = %#v, missing %q", fake.args, want)
			}
		}
	})

	t.Run("missing SSL choice", func(t *testing.T) {
		fake := &fakeRun{exitCode: 1, stderr: "official validation error"}
		result, _, _ := execute(t, fake, testAccessToken,
			"ssl-enforcement", "update", "--project-ref", "abcdefghijklmnopqrst",
		)
		if result.ExitCode != 1 {
			t.Errorf("result = %+v, want official CLI exit 1", result)
		}
		if !fake.called {
			t.Fatal("official CLI was not called")
		}
	})
}

// TestFunctionBundlerSelectionIsExplicit keeps the official Docker/API choice
// unchanged unless the caller opts into server-side bundling.
func TestFunctionBundlerSelectionIsExplicit(t *testing.T) {
	t.Run("official default", func(t *testing.T) {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken,
			"functions", "deploy", "hello", "goodbye",
			"--project-ref", "abcdefghijklmnopqrst",
			"--workdir", "/workspace/app",
			"--prune",
		)
		if result.ExitCode != 0 {
			t.Fatalf("result = %+v, want success", result)
		}
		for _, unwanted := range []string{"--use-api", "--use-api=true"} {
			if slices.Contains(fake.args, unwanted) {
				t.Errorf("argv = %#v, unexpectedly contains %q", fake.args, unwanted)
			}
		}
	})

	t.Run("server-side API opt in", func(t *testing.T) {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken,
			"functions", "deploy", "hello",
			"--project-ref", "abcdefghijklmnopqrst",
			"--use-api",
			"--jobs", "3",
		)
		if result.ExitCode != 0 {
			t.Fatalf("result = %+v, want success", result)
		}
		for _, want := range []string{"--use-api", "--jobs", "3"} {
			if !slices.Contains(fake.args, want) {
				t.Errorf("argv = %#v, missing %q", fake.args, want)
			}
		}
	})
}

// TestOfficialGlobalFlagsAreOptIn verifies that confirmation and experimental
// gates are controlled by the caller rather than silently enabled.
func TestOfficialGlobalFlagsAreOptIn(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken, "projects", "list")
		if result.ExitCode != 0 {
			t.Fatalf("result = %+v, want success", result)
		}
		for _, unwanted := range []string{"--experimental", "--experimental=true", "--yes", "--yes=true"} {
			if slices.Contains(fake.args, unwanted) {
				t.Errorf("argv = %#v, unexpectedly contains %q", fake.args, unwanted)
			}
		}
	})

	t.Run("explicit values", func(t *testing.T) {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken,
			"projects", "delete", "aaaaaaaaaaaaaaaaaaaa",
			"--experimental", "--yes",
		)
		if result.ExitCode != 0 {
			t.Fatalf("result = %+v, want success", result)
		}
		for _, want := range []string{"--experimental", "--yes"} {
			if !slices.Contains(fake.args, want) {
				t.Errorf("argv = %#v, missing %q", fake.args, want)
			}
		}
	})
}

// TestOfficialOptInFlagsAppearInHelp keeps caller-controlled gates
// discoverable without making them wrapper validation rules.
func TestOfficialOptInFlagsAppearInHelp(t *testing.T) {
	var stdout bytes.Buffer
	root := (&Service{Out: &stdout}).NewCommandTree()
	root.SetArgs([]string{"projects", "delete", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("projects delete --help: %v", err)
	}
	for _, flag := range []string{"--experimental", "--yes"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Errorf("help = %q, missing %s", stdout.String(), flag)
		}
	}
}

// TestGenTypesHelpDocumentsOfficialTargets keeps every forwarded generation
// target discoverable without claiming that AnyCLI restricts provider modes.
func TestGenTypesHelpDocumentsOfficialTargets(t *testing.T) {
	var stdout bytes.Buffer
	root := (&Service{Out: &stdout}).NewCommandTree()
	root.SetArgs([]string{"gen", "types", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("gen types --help: %v", err)
	}
	help := stdout.String()
	for _, flag := range []string{"--local", "--linked", "--project-id", "--db-url"} {
		if !strings.Contains(help, flag) {
			t.Errorf("help = %q, missing %s", help, flag)
		}
	}
	if strings.Contains(help, "unavailable") {
		t.Errorf("help = %q, unexpectedly restricts official generation targets", help)
	}
}

// TestGenTypesDelegatesProjectIDValidation verifies AnyCLI forwards generation
// arguments and leaves required-input validation to the official CLI.
func TestGenTypesDelegatesProjectIDValidation(t *testing.T) {
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
		"--project-id", "abcdefghijklmnopqrst",
		"--lang", "go",
		"--schema", "public,auth",
	} {
		if !slices.Contains(fake.args, want) {
			t.Errorf("argv = %#v, missing %q", fake.args, want)
		}
	}

	missing := &fakeRun{exitCode: 1, stderr: "official validation error"}
	missingResult, _, _ := execute(t, missing, testAccessToken, "gen", "types")
	if missingResult.ExitCode != 1 || !missing.called {
		t.Errorf("missing project id: result=%+v called=%v, want official validation", missingResult, missing.called)
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
				"--linked", "--project-ref", "abcdefghijklmnopqrst",
				"--output", "json", "--output-format", "json", "--agent", "yes",
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
				"--db-url", "postgresql://user:password@example.test:5432/postgres",
				"--file", "queries/report.sql", "--workdir", "/workspace/app",
				"--output", "json", "--output-format", "json", "--agent", "yes",
			},
		},
		{
			name: "explicit local target",
			args: []string{"db", "query", "select 1", "--local"},
			wantArgs: []string{
				"db", "query", "select 1", "--local",
				"--output", "json", "--output-format", "json", "--agent", "yes",
			},
		},
		{
			name: "official default target",
			args: []string{"db", "query", "select 1"},
			wantArgs: []string{
				"db", "query", "select 1",
				"--output", "json", "--output-format", "json", "--agent", "yes",
			},
		},
		{
			name: "argument terminator before SQL comment",
			args: []string{"db", "query", "--", "-- comment\nselect 1"},
			wantArgs: []string{
				"db", "query",
				"--output", "json", "--output-format", "json", "--agent", "yes",
				"--", "-- comment\nselect 1",
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

// TestStorageCopyUsesTheOfficialCommand ensures both copy directions share one
// side-effectful command and leave their policy decision to the embedding host.
func TestStorageCopyUsesTheOfficialCommand(t *testing.T) {
	for _, paths := range [][]string{
		{"ss:///assets/docs", "./docs"},
		{"./local", "ss:///assets/file"},
	} {
		fake := &fakeRun{}
		result, _, _ := execute(t, fake, testAccessToken,
			"storage", "cp", paths[0], paths[1],
			"--project-ref", "abcdefghijklmnopqrst",
			"--recursive",
			"--experimental",
		)
		if result.ExitCode != 0 {
			t.Fatalf("paths %v: result = %+v, want success", paths, result)
		}
		wantArgs := []string{
			"storage", "cp", paths[0], paths[1],
			"--project-ref", "abcdefghijklmnopqrst",
			"--recursive",
			"--experimental",
			"--output", "json",
			"--output-format", "json",
			"--agent", "yes",
		}
		if !slices.Equal(fake.args, wantArgs) {
			t.Errorf("paths %v: argv = %#v, want %#v", paths, fake.args, wantArgs)
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

// TestRootHelpKeepsTheFlattenedFaceLean prevents provider prose from pushing
// the exhaustive runnable-command list down in the generated root help.
func TestRootHelpKeepsTheFlattenedFaceLean(t *testing.T) {
	root := (&Service{}).NewCommandTree()
	if root.Long != "" {
		t.Errorf("root Long = %q, want empty", root.Long)
	}
}

// TestCommandSurfaceAndSideEffects pins the complete public face. Any command
// not listed here is intentionally unavailable in this token-only wrapper.
func TestCommandSurfaceAndSideEffects(t *testing.T) {
	want := map[string]string{
		"backups list":                "false", // GET /v1/projects/{ref}/database/backups
		"branches create":             "true",  // POST /v1/projects/{ref}/branches
		"branches delete":             "true",  // DELETE /v1/branches/{branch-id-or-ref}
		"branches list":               "false", // GET /v1/projects/{ref}/branches
		"branches pause":              "true",  // POST /v1/projects/{branch-ref}/pause
		"branches unpause":            "true",  // POST /v1/projects/{branch-ref}/restore
		"branches update":             "true",  // PATCH /v1/branches/{branch-id-or-ref}
		"db query":                    "true",  // Arbitrary SQL via Management API or a direct/local Postgres connection
		"domains get":                 "false", // GET /v1/projects/{ref}/custom-hostname
		"functions delete":            "true",  // DELETE /v1/projects/{ref}/functions/{slug}
		"functions deploy":            "true",  // POST/PATCH/DELETE /v1/projects/{ref}/functions endpoints
		"functions download":          "false", // GET function metadata and bundle endpoints
		"functions list":              "false", // GET /v1/projects/{ref}/functions
		"gen types":                   "false", // GET /v1/projects/{ref}/types/{language}
		"network-bans get":            "false", // POST /v1/projects/{ref}/network-bans/retrieve (read-only operation)
		"network-bans remove":         "true",  // DELETE /v1/projects/{ref}/network-bans
		"network-restrictions get":    "false", // GET /v1/projects/{ref}/network-restrictions
		"network-restrictions update": "true",  // POST apply or PATCH /v1/projects/{ref}/network-restrictions
		"orgs list":                   "false", // GET /v1/organizations
		"postgres-config delete":      "true",  // GET then PUT /v1/projects/{ref}/config/database/postgres
		"postgres-config get":         "false", // GET /v1/projects/{ref}/config/database/postgres
		"postgres-config update":      "true",  // PUT /v1/projects/{ref}/config/database/postgres
		"projects delete":             "true",  // DELETE /v1/projects/{ref}
		"projects list":               "false", // GET /v1/projects
		"secrets list":                "false", // GET /v1/projects/{ref}/secrets
		"snippets download":           "false", // GET /v1/snippets/{snippet-id}
		"snippets list":               "false", // GET /v1/snippets?project_ref={ref}
		"ssl-enforcement get":         "false", // GET /v1/projects/{ref}/ssl-enforcement
		"ssl-enforcement update":      "true",  // PUT /v1/projects/{ref}/ssl-enforcement
		"sso info":                    "false", // Derives service-provider values locally from the project ref
		"sso list":                    "false", // GET /v1/projects/{ref}/config/auth/sso/providers
		"sso show":                    "false", // GET /v1/projects/{ref}/config/auth/sso/providers/{provider-id}
		"storage cp":                  "true",  // Copies in either direction and may mutate remote Storage
		"storage ls":                  "false", // GET buckets or POST /storage/v1/object/list/{bucket} (read-only operation)
		"vanity-subdomains get":       "false", // GET /v1/projects/{ref}/vanity-subdomain
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
