// Package supabase wraps the official Supabase CLI with an explicit,
// token-bound command tree. Database queries retain the official target
// semantics; local stack lifecycle, migrations, login, and arbitrary
// passthrough commands are intentionally absent.
package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/config"
	"github.com/heliohq/anycli/internal/exec/binresolve"
	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvAccessToken is the documented Supabase CLI environment variable bound
// by definitions/tools/supabase.json.
const EnvAccessToken = "SUPABASE_ACCESS_TOKEN"

const (
	defaultTimeout = 15 * time.Minute
	sideEffectKey  = "anycli.side_effect"
)

// Runner executes one official Supabase CLI subprocess. Tests replace it to
// inspect argv and environment without making provider calls.
type Runner func(ctx context.Context, args, env []string) (exitCode int, stdout, stderr []byte, err error)

// Service implements the built-in Supabase tool.
type Service struct {
	// Run overrides subprocess execution; nil resolves and executes the pinned
	// official Supabase CLI.
	Run Runner
	// Out and Err override process output streams.
	Out io.Writer
	Err io.Writer
	// HC carries the optional engine-level HTTP client used for lazy install.
	HC *http.Client
}

// invocation carries subprocess state across Cobra's RunE boundary so Execute
// can preserve runtime exit codes and distinguish usage failures.
type invocation struct {
	started  bool
	exitCode int
}

// flagKind selects the Cobra parser used for one exposed official CLI flag.
type flagKind string

const (
	flagString flagKind = "string"
	flagBool   flagKind = "bool"
	flagInt    flagKind = "int"
)

// flagSpec declares one allowed official CLI flag. Flags absent from these
// specs cannot reach the subprocess.
type flagSpec struct {
	Name     string
	Kind     flagKind
	Usage    string
	Required bool
}

// commandSpec declares one runnable leaf and its fixed official CLI mapping.
type commandSpec struct {
	Group      string
	Use        string
	Short      string
	Long       string
	Args       cobra.PositionalArgs
	SideEffect bool
	Flags      []flagSpec
	BinaryPath []string
	FixedArgs  []string
}

// groupSpec supplies help for a non-runnable command group.
type groupSpec struct {
	Name  string
	Short string
}

// cliErrorEnvelope is the official CLI's --output-format=json error shape.
type cliErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var groupSpecs = []groupSpec{
	{Name: "backups", Short: "Inspect physical backups"},
	{Name: "branches", Short: "Manage preview branches"},
	{Name: "db", Short: "Run database queries"},
	{Name: "domains", Short: "Inspect custom domains"},
	{Name: "functions", Short: "Manage Edge Functions through the remote API"},
	{Name: "gen", Short: "Generate project artifacts through remote APIs"},
	{Name: "network-bans", Short: "Inspect and remove project network bans"},
	{Name: "network-restrictions", Short: "Inspect and update project network restrictions"},
	{Name: "orgs", Short: "Inspect organizations"},
	{Name: "postgres-config", Short: "Inspect and update Postgres configuration"},
	{Name: "projects", Short: "Inspect and manage projects"},
	{Name: "secrets", Short: "Inspect project secret metadata"},
	{Name: "snippets", Short: "Inspect SQL snippets"},
	{Name: "ssl-enforcement", Short: "Inspect and update database SSL enforcement"},
	{Name: "sso", Short: "Inspect project SAML SSO configuration"},
	{Name: "storage", Short: "Read Storage objects"},
	{Name: "vanity-subdomains", Short: "Inspect vanity subdomains"},
}

var commandSpecs = []commandSpec{
	{
		Group: "backups", Use: "list", Short: "List physical backups for a project",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "branches", Use: "list", Short: "List preview branches for a project",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "branches", Use: "create <name>", Short: "Create a preview branch",
		Args: cobra.ExactArgs(1), SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("region", "Region for the branch database", false),
			stringFlag("size", "Instance size for the branch database", false),
			boolFlag("persistent", "Create a persistent branch"),
			boolFlag("with-data", "Clone production data into the branch"),
			stringFlag("notify-url", "URL notified when the branch is healthy", false),
			stringFlag("git-branch", "Git branch associated with the preview branch", false),
		},
	},
	{
		Group: "branches", Use: "update <name-or-id>", Short: "Update a preview branch",
		Args: cobra.ExactArgs(1), SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("name", "New preview branch name", false),
			stringFlag("git-branch", "New associated Git branch", false),
			boolFlag("persistent", "Set persistent state; accepts --persistent=false"),
			stringFlag("status", "Override preview branch status", false),
			stringFlag("notify-url", "URL notified when the branch is healthy", false),
		},
	},
	{
		Group: "branches", Use: "pause <name-or-id>", Short: "Pause a preview branch",
		Args: cobra.ExactArgs(1), SideEffect: true, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "branches", Use: "unpause <name-or-id>", Short: "Unpause a preview branch",
		Args: cobra.ExactArgs(1), SideEffect: true, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "branches", Use: "delete <name-or-id>", Short: "Delete a preview branch",
		Args: cobra.ExactArgs(1), SideEffect: true, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "db", Use: "query [sql]", Short: "Execute SQL against a Supabase database",
		Long: "Preserves the official CLI's linked, local, and direct database target semantics. Every query is classified as side-effectful so embedding hosts can require approval.",
		Args: cobra.MaximumNArgs(1), SideEffect: true,
		Flags: []flagSpec{
			stringFlag("db-url", "Postgres connection string", false),
			boolFlag("linked", "Query a linked Supabase project"),
			boolFlag("local", "Query the local Supabase database"),
			stringFlag("project-ref", "Project ref used with --linked", false),
			stringFlag("file", "Read SQL from a file", false),
			stringFlag("workdir", "Supabase project directory used for local state and relative paths", false),
		},
	},
	{
		Group: "domains", Use: "get", Short: "Get custom domain configuration",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "functions", Use: "list", Short: "List deployed Edge Functions",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "functions", Use: "download [name]", Short: "Download one or all Edge Functions without Docker",
		Long: "Downloads through Supabase's server-side API bundler; Docker is never used.",
		Args: cobra.MaximumNArgs(1),
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("workdir", "Supabase project directory receiving the files", false),
		},
		FixedArgs: []string{"--use-api"},
	},
	{
		Group: "functions", Use: "deploy [name...]", Short: "Deploy Edge Functions without Docker",
		Long: "Bundles through Supabase's server-side API; Docker is never used.",
		Args: cobra.ArbitraryArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("workdir", "Supabase project directory containing the functions", false),
			boolFlag("no-verify-jwt", "Disable JWT verification for the deployed functions"),
			stringFlag("import-map", "Path to an import map", false),
			boolFlag("prune", "Delete remote functions that are absent locally"),
			intFlag("jobs", "Maximum parallel deployment jobs"),
		},
		FixedArgs: []string{"--use-api"},
	},
	{
		Group: "functions", Use: "delete <name>", Short: "Delete a deployed Edge Function",
		Args: cobra.ExactArgs(1), SideEffect: true, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "gen", Use: "types", Short: "Generate schema types through the Management API",
		Long: "Requires --project-id; local, linked, and direct database modes are unavailable.",
		Args: cobra.NoArgs,
		Flags: []flagSpec{
			stringFlag("project-id", "Project ref used by the Management API", true),
			stringFlag("lang", "Output language: typescript, go, swift, or python", false),
			stringFlag("schema", "Comma-separated schemas to include", false),
			stringFlag("swift-access-control", "Swift access control: internal or public", false),
			boolFlag("postgrest-v9-compat", "Generate types compatible with PostgREST v9 and below"),
			stringFlag("query-timeout", "Maximum schema query timeout", false),
		},
	},
	{
		Group: "network-bans", Use: "get", Short: "Get current project network bans",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "network-bans", Use: "remove", Short: "Remove a project network ban",
		Args: cobra.NoArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("db-unban-ip", "IPv4 address to unban", true),
		},
	},
	{
		Group: "network-restrictions", Use: "get", Short: "Get current project network restrictions",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "network-restrictions", Use: "update", Short: "Update project network restrictions",
		Args: cobra.NoArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("db-allow-cidr", "CIDR allowed to connect to Postgres", true),
			boolFlag("bypass-cidr-checks", "Bypass selected CIDR validation checks"),
			boolFlag("append", "Append instead of replacing current restrictions"),
		},
	},
	{
		Group: "orgs", Use: "list", Short: "List organizations available to the OAuth grant",
		Args: cobra.NoArgs,
	},
	{
		Group: "postgres-config", Use: "get", Short: "Get Postgres configuration overrides",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "postgres-config", Use: "update", Short: "Update Postgres configuration overrides",
		Args: cobra.NoArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("config", "Configuration override as key=value", true),
			boolFlag("replace-existing-overrides", "Replace all existing overrides"),
			boolFlag("no-restart", "Do not restart Postgres after the update"),
		},
	},
	{
		Group: "postgres-config", Use: "delete", Short: "Delete Postgres configuration overrides",
		Args: cobra.NoArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), stringFlag("config", "Comma-separated configuration keys to delete", true),
			boolFlag("no-restart", "Do not restart Postgres after deletion"),
		},
	},
	{
		Group: "projects", Use: "list", Short: "List projects available to the OAuth grant",
		Args: cobra.NoArgs,
	},
	{
		Group: "projects", Use: "delete <project-ref>", Short: "Delete a project",
		Args: cobra.ExactArgs(1), SideEffect: true,
	},
	{
		Group: "secrets", Use: "list", Short: "List Edge Function secret names and digests",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "snippets", Use: "list", Short: "List SQL snippets for a project",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "snippets", Use: "download <snippet-id>", Short: "Download one SQL snippet",
		Args: cobra.ExactArgs(1), Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "ssl-enforcement", Use: "get", Short: "Get database SSL enforcement configuration",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "ssl-enforcement", Use: "update", Short: "Update database SSL enforcement configuration",
		Args: cobra.NoArgs, SideEffect: true,
		Flags: []flagSpec{
			projectRefFlag(), boolFlag("enable-db-ssl-enforcement", "Enable SSL enforcement"),
			boolFlag("disable-db-ssl-enforcement", "Disable SSL enforcement"),
		},
	},
	{
		Group: "sso", Use: "list", Short: "List SAML SSO identity providers",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "sso", Use: "show <provider-id>", Short: "Show one SAML SSO identity provider",
		Args:  cobra.ExactArgs(1),
		Flags: []flagSpec{projectRefFlag(), boolFlag("metadata", "Return raw SAML metadata")},
	},
	{
		Group: "sso", Use: "info", Short: "Get project SAML service-provider information",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
	{
		Group: "storage", Use: "ls [ss:///bucket/prefix]", Short: "List Storage objects",
		Long:      "Uses the official experimental Storage command against an explicit remote project.",
		Args:      cobra.MaximumNArgs(1),
		Flags:     []flagSpec{projectRefFlag(), boolFlag("recursive", "List objects recursively")},
		FixedArgs: []string{"--experimental"},
	},
	{
		Group: "storage", Use: "download <ss:///source> <local-destination>", Short: "Download Storage objects",
		Long: "Maps to official `storage cp` with remote-to-local arguments only; upload and remote mutation are unavailable.",
		Args: remoteToLocalArgs,
		Flags: []flagSpec{
			projectRefFlag(), boolFlag("recursive", "Download a directory recursively"),
			intFlag("jobs", "Maximum parallel download jobs"),
		},
		BinaryPath: []string{"storage", "cp"}, FixedArgs: []string{"--experimental"},
	},
	{
		Group: "vanity-subdomains", Use: "get", Short: "Get vanity subdomain configuration",
		Args: cobra.NoArgs, Flags: []flagSpec{projectRefFlag()},
	},
}

// projectRefFlag returns the required explicit project selector shared by
// project-scoped commands, preventing fallback to linked local state.
func projectRefFlag() flagSpec {
	return stringFlag("project-ref", "Supabase project ref", true)
}

// stringFlag declares one string-valued official CLI flag.
func stringFlag(name, usage string, required bool) flagSpec {
	return flagSpec{Name: name, Kind: flagString, Usage: usage, Required: required}
}

// boolFlag declares one boolean official CLI flag.
func boolFlag(name, usage string) flagSpec {
	return flagSpec{Name: name, Kind: flagBool, Usage: usage}
}

// intFlag declares one integer official CLI flag.
func intFlag(name, usage string) flagSpec {
	return flagSpec{Name: name, Kind: flagInt, Usage: usage}
}

// remoteToLocalArgs validates the constrained Storage copy direction before
// any subprocess can run.
func remoteToLocalArgs(_ *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(2)(nil, args); err != nil {
		return err
	}
	if !strings.HasPrefix(args[0], "ss:///") {
		return errors.New("source must be a remote Supabase Storage path starting with ss:///")
	}
	if strings.HasPrefix(args[1], "ss://") {
		return errors.New("destination must be a local path")
	}
	return nil
}

// Execute parses and runs one allowed Supabase command with the resolved OAuth
// token injected only into the child environment.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), EnvAccessToken+" is not set")
		return execution.Result{ExitCode: 1}, nil
	}

	inv := &invocation{}
	root := s.newRoot(token, inv)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), redactSecret(err.Error(), token))
		exitCode := 2
		if inv.started {
			exitCode = inv.exitCode
			if exitCode <= 0 {
				exitCode = 1
			}
		}
		return execution.Result{
			ExitCode:           exitCode,
			CredentialRejected: execution.IsCredentialRejected(err),
		}, nil
	}
	return execution.Result{ExitCode: inv.exitCode}, nil
}

// newRoot builds the explicit command tree shared by execution, help,
// inspection, and side-effect policy traversal.
func (s *Service) newRoot(token string, inv *invocation) *cobra.Command {
	pin := pinnedSupabaseVersion()
	root := &cobra.Command{
		Use:           "supabase",
		Short:         fmt.Sprintf("Selected Supabase operations via official CLI %s", pin),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	timeout := root.PersistentFlags().Duration("timeout", defaultTimeout, "official CLI execution timeout")

	groups := make(map[string]*cobra.Command, len(groupSpecs))
	for _, spec := range groupSpecs {
		group := &cobra.Command{
			Use:   spec.Name,
			Short: spec.Short,
			RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		}
		groups[spec.Name] = group
		root.AddCommand(group)
	}
	for i := range commandSpecs {
		spec := commandSpecs[i]
		groups[spec.Group].AddCommand(s.newLeaf(spec, token, timeout, inv))
	}
	return root
}

// newLeaf converts one declarative command specification into a Cobra leaf
// and a fixed official CLI argv mapping.
func (s *Service) newLeaf(spec commandSpec, token string, timeout *time.Duration, inv *invocation) *cobra.Command {
	command := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  spec.Args,
		Annotations: map[string]string{
			sideEffectKey: fmt.Sprintf("%t", spec.SideEffect),
		},
		RunE: func(cmd *cobra.Command, positional []string) error {
			inv.started = true
			return s.runSupabase(cmd.Context(), officialArgs(cmd, spec, positional), token, *timeout, inv)
		},
	}
	for _, flag := range spec.Flags {
		switch flag.Kind {
		case flagString:
			command.Flags().String(flag.Name, "", flag.Usage)
		case flagBool:
			command.Flags().Bool(flag.Name, false, flag.Usage)
		case flagInt:
			command.Flags().Int(flag.Name, 0, flag.Usage)
		}
		if flag.Required {
			_ = command.MarkFlagRequired(flag.Name)
		}
	}
	return command
}

// officialArgs assembles the complete execve argv from a leaf's fixed path,
// positional arguments, explicitly exposed flags, and non-interactive output
// controls.
func officialArgs(command *cobra.Command, spec commandSpec, positional []string) []string {
	path := spec.BinaryPath
	if len(path) == 0 {
		path = []string{spec.Group, command.Name()}
	}
	args := append(slices.Clone(path), positional...)
	for _, exposed := range spec.Flags {
		flag := command.Flags().Lookup(exposed.Name)
		if flag != nil && flag.Changed {
			args = append(args, "--"+exposed.Name+"="+flag.Value.String())
		}
	}
	args = append(args, spec.FixedArgs...)
	return append(args,
		"--output", "json",
		"--output-format", "json",
		"--agent", "yes",
		"--yes",
	)
}

// runSupabase resolves the official binary, isolates its credential state,
// executes it with a bounded context, and classifies credential failures.
func (s *Service) runSupabase(ctx context.Context, args []string, token string, timeout time.Duration, inv *invocation) error {
	run := s.Run
	if run == nil {
		binary, err := s.resolveSupabaseBinary(ctx)
		if err != nil {
			return err
		}
		run = supabaseRunner(binary)
	}

	scopedHome, err := os.MkdirTemp("", "anycli-supabase-home-") //anycli:scratch — isolates CLI auth, profile, and telemetry state
	if err != nil {
		return fmt.Errorf("create isolated Supabase CLI home: %w", err)
	}
	defer os.RemoveAll(scopedHome) //anycli:scratch — removes the isolated CLI state above

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	exitCode, stdout, stderr, runErr := run(ctx, args, childEnv(token, scopedHome))
	inv.exitCode = exitCode
	if len(stdout) > 0 {
		_, _ = s.stdout().Write([]byte(redactSecret(string(stdout), token)))
	}
	if len(stderr) > 0 {
		_, _ = s.stderr().Write([]byte(redactSecret(string(stderr), token)))
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("Supabase CLI timed out after %s; adjust with --timeout", timeout)
	}
	if runErr != nil {
		return classifyFailure(runErr, stdout, stderr)
	}
	if exitCode != 0 {
		return classifyFailure(fmt.Errorf("Supabase CLI exited with code %d", exitCode), stdout, stderr)
	}
	return nil
}

// childEnv removes ambient Supabase and database credentials, then installs
// the one resolved OAuth token and an isolated, telemetry-disabled CLI home.
func childEnv(token, scopedHome string) []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent)+8)
	for _, binding := range parent {
		key, _, _ := strings.Cut(binding, "=")
		if strings.HasPrefix(key, "SUPABASE_") {
			continue
		}
		switch key {
		case "HOME", "USERPROFILE", "DO_NOT_TRACK", "DB_PASSWORD", "PGPASSWORD", "DATABASE_URL":
			continue
		}
		env = append(env, binding)
	}
	return append(env,
		"HOME="+scopedHome,
		"USERPROFILE="+scopedHome,
		"SUPABASE_HOME="+scopedHome,
		EnvAccessToken+"="+token,
		"SUPABASE_NO_KEYRING=1",
		"SUPABASE_NO_UPDATE_NOTIFIER=1",
		"SUPABASE_TELEMETRY_DISABLED=1",
		"DO_NOT_TRACK=1",
	)
}

// resolveSupabaseBinary lazily installs the pinned official CLI release while
// keeping installation notices on the service's stderr stream.
func (s *Service) resolveSupabaseBinary(ctx context.Context) (string, error) {
	definition, err := definitions.LoadBundled("supabase")
	if err != nil {
		return "", err
	}
	options := binresolve.Options{SkipPATHDir: config.BinDir(), Notice: s.stderr()}
	if s.HC != nil {
		options.Downloader = binresolve.HTTPDownloader(s.HC)
	}
	binary, err := binresolve.Resolve(ctx, definition.Name, definition.Binary, definition.Source, options)
	if err != nil {
		return "", fmt.Errorf("resolve Supabase CLI: %w", err)
	}
	return binary, nil
}

// supabaseRunner executes the resolved official CLI with no shell and no
// interactive stdin.
func supabaseRunner(binary string) Runner {
	return func(ctx context.Context, args, env []string) (int, []byte, []byte, error) {
		command := exec.CommandContext(ctx, binary, args...)
		command.Env = env
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		runErr := command.Run()

		exitCode := 1
		if command.ProcessState != nil {
			exitCode = command.ProcessState.ExitCode()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			runErr = nil
		}
		return exitCode, stdout.Bytes(), stderr.Bytes(), runErr
	}
}

// classifyFailure marks explicit access-token rejection while preserving
// authorization/scope failures as ordinary provider errors.
func classifyFailure(err error, stdout, stderr []byte) error {
	for _, output := range [][]byte{stdout, stderr} {
		var envelope cliErrorEnvelope
		if json.Unmarshal(bytes.TrimSpace(output), &envelope) == nil {
			code := strings.ToLower(envelope.Error.Code)
			message := strings.ToLower(envelope.Error.Message)
			if strings.Contains(code, "invalidaccesstoken") ||
				strings.Contains(code, "authrequired") ||
				strings.Contains(message, "invalid access token") ||
				strings.Contains(message, `"message":"unauthorized"`) ||
				strings.TrimSpace(message) == "unauthorized" {
				return execution.RejectCredential(err)
			}
		}
	}
	return err
}

// redactSecret removes the OAuth access token from all subprocess output and
// wrapper errors before they reach the caller.
func redactSecret(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

// NewCommandTree returns the full dry-run tree used by help, inspection, lint,
// and approval policy without resolving credentials or the binary.
func (s *Service) NewCommandTree() *cobra.Command {
	return s.newRoot("", &invocation{})
}

// stdout returns the configured service output stream.
func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

// stderr returns the configured service error stream.
func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// pinnedSupabaseVersion reads the release pin from the embedded definition
// once for root help text.
var pinnedSupabaseVersion = sync.OnceValue(func() string {
	definition, err := definitions.LoadBundled("supabase")
	if err != nil || definition.Source == nil {
		return "(unpinned)"
	}
	return definition.Source.Version
})
