package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shipbase/anycli/definitions"
	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/credential"
	"github.com/shipbase/anycli/internal/middleware"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/shipbase/anycli/internal/tools"
)

// loadDefinition loads a tool definition by name. It is a package variable so
// tests can inject synthetic definitions; production always loads from the
// embedded definition set.
var loadDefinition = definitions.LoadBundled

// Engine runs tools through the full credential + middleware pipeline. It holds
// the consumer-supplied credential cache; tool definitions come from the
// internal embedded set, not from the engine's configuration.
type Engine struct {
	cache credential.Cache
}

// NewEngine constructs an Engine with the given credential cache. cache must be
// non-nil; the public constructor installs the in-memory default when the
// consumer supplies none.
func NewEngine(cache credential.Cache) (*Engine, error) {
	if cache == nil {
		return nil, fmt.Errorf("credential cache must not be nil")
	}
	return &Engine{cache: cache}, nil
}

// Execute runs a tool through the full credential + middleware pipeline.
//
// The tool's definition is loaded from the embedded definition set; an unknown
// tool (no embedded definition) returns an error. Credentials come from the
// supplied resolver, which must be non-nil. Resolver-supplied credentials are
// always treated as ephemeral/managed (file injection writes a temp file and
// redirects via config_env / config_flag, then cleans up).
func (e *Engine) Execute(ctx context.Context, tool string, args []string, resolver credential.CredentialResolver) (int, error) {
	if resolver == nil {
		return 1, fmt.Errorf("credential resolver must not be nil")
	}

	// 1. Load tool definition from the embedded set.
	def, err := loadDefinition(tool)
	if err != nil {
		return 1, err
	}

	mctx := &middleware.Context{
		Args: args,
		Env:  make(map[string]string),
	}

	// Track whether this tool has any credential bindings, for stale marking.
	hasCredentials := def.Auth != nil && len(def.Auth.Credentials) > 0

	// 2-3. Resolve credentials and apply bindings.
	if hasCredentials {
		values, err := credential.ResolveBindings(ctx, e.cache, tool, def.Auth.Credentials, resolver)
		if err != nil {
			return 1, fmt.Errorf("credential resolution failed for %q: %w", tool, err)
		}

		injResult, err := credential.ApplyBindings(tool, def.Auth.Credentials, values)
		if err != nil {
			return 1, fmt.Errorf("credential injection failed for %q: %w", tool, err)
		}

		// Defer cleanup for ephemeral temp files (file inject).
		if injResult.Cleanup != nil {
			defer injResult.Cleanup()
		}

		// Merge injected env vars into mctx.Env.
		for k, v := range injResult.Env {
			mctx.Env[k] = v
		}

		// Append injected args (after user args, for subcommand-scoped flags).
		if len(injResult.Args) > 0 {
			mctx.Args = append(mctx.Args, injResult.Args...)
		}
	}

	// 4. If service type, delegate to built-in service.
	if def.Type == "service" {
		svc, err := tools.GetService(tool)
		if err != nil {
			return 1, err
		}
		exitCode, err := svc.Execute(ctx, mctx.Args, mctx.Env)
		if exitCode != 0 && hasCredentials {
			e.markCredentialsStale(tool)
		}
		return exitCode, err
	}

	// 5. Resolve the real binary path.
	binaryPath, err := resolveBinary(def)
	if err != nil {
		return 1, fmt.Errorf("cannot find %q binary: %w", tool, err)
	}

	// 6. Run before hooks.
	if err := middleware.RunBefore(def.Before, mctx); err != nil {
		return 1, err
	}

	// 7-8. Execute binary.
	// If no after hooks, passthrough stdin/stdout/stderr directly (streaming).
	if len(def.After) == 0 {
		exitCode, err := executePassthrough(binaryPath, mctx.Args, mctx.Env)
		// 9. On non-zero exit, mark credentials stale.
		if exitCode != 0 && hasCredentials {
			e.markCredentialsStale(tool)
		}
		return exitCode, err
	}

	// With after hooks, capture output for processing.
	mctx.ExitCode, mctx.Stdout, mctx.Stderr, err = executeBuffered(binaryPath, mctx.Args, mctx.Env)
	rawExitCode := mctx.ExitCode // save before after-hooks can remap
	if err != nil && mctx.ExitCode == 0 {
		return 1, err
	}

	// 8. Run after hooks.
	if err := middleware.RunAfter(def.After, mctx); err != nil {
		return mctx.ExitCode, err
	}

	os.Stdout.Write(mctx.Stdout)
	os.Stderr.Write(mctx.Stderr)

	// 9. On non-zero raw exit (before after-hook remapping), mark credentials stale.
	if rawExitCode != 0 && hasCredentials {
		e.markCredentialsStale(tool)
	}

	return mctx.ExitCode, nil
}

// markCredentialsStale marks the cached credential for a tool as stale and
// prints a hint to stderr so the agent retries (triggering a re-resolve).
func (e *Engine) markCredentialsStale(tool string) {
	e.cache.MarkStale(tool)
	fmt.Fprintf(os.Stderr, "[anycli] credentials for %q may be stale. retry the same command to fetch fresh credentials.\n", tool)
}

// resolveBinary finds the real binary path, skipping the anycli shim directory.
func resolveBinary(def *registry.Definition) (string, error) {
	if def.Resolve != "" && def.Resolve != "which" {
		// Absolute path provided
		if _, err := os.Stat(def.Resolve); err != nil {
			return "", err
		}
		return def.Resolve, nil
	}

	// Search PATH, but skip our own shim directory
	shimDir := config.BinDir()
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == shimDir {
			continue
		}
		candidate := filepath.Join(dir, def.Binary)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH", def.Binary)
}

// executePassthrough runs the binary with stdin/stdout/stderr connected directly.
func executePassthrough(binary string, args []string, env map[string]string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = buildEnv(env)

	err := cmd.Run()
	return cmd.ProcessState.ExitCode(), err
}

// executeBuffered runs the binary and captures output for after hooks.
func executeBuffered(binary string, args []string, env map[string]string) (int, []byte, []byte, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = buildEnv(env)

	err := cmd.Run()
	return cmd.ProcessState.ExitCode(), stdout.Bytes(), stderr.Bytes(), err
}

func buildEnv(env map[string]string) []string {
	result := os.Environ()
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
