package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/credential"
	"github.com/shipbase/anycli/internal/middleware"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/shipbase/anycli/internal/tools"
)

// Run executes a tool through the full credential + middleware pipeline.
func Run(name string, args []string) (int, error) {
	// 1. Load tool definition
	def, err := registry.Load(name)
	if err != nil {
		return 1, err
	}

	ctx := &middleware.Context{
		Args: args,
		Env:  make(map[string]string),
	}

	// Collect unique vault_tool values for potential stale marking later.
	var vaultTools []string

	// 2-3. Resolve credentials and apply bindings
	if def.Auth != nil && len(def.Auth.Credentials) > 0 {
		// Resolve credentials (vault -> local -> skip)
		values, err := credential.Resolve(name, def.Auth.Credentials)
		if err != nil {
			return 1, fmt.Errorf("credential resolution failed for %q: %w", name, err)
		}

		// Apply bindings (env/arg/file)
		injResult, err := credential.ApplyBindings(name, def.Auth.Credentials, values, credential.IsVaultMode())
		if err != nil {
			return 1, fmt.Errorf("credential injection failed for %q: %w", name, err)
		}

		// Defer cleanup for temp files (vault mode file inject)
		if injResult.Cleanup != nil {
			defer injResult.Cleanup()
		}

		// Merge injected env vars into ctx.Env
		for k, v := range injResult.Env {
			ctx.Env[k] = v
		}

		// Prepend injected args
		if len(injResult.Args) > 0 {
			ctx.Args = append(injResult.Args, ctx.Args...)
		}

		// Collect unique vault_tool values for stale marking on failure
		vaultTools = uniqueVaultTools(def.Auth.Credentials)
	}

	// 4. If service type, delegate to built-in service
	if def.Type == "service" {
		svc, err := tools.GetService(name)
		if err != nil {
			return 1, err
		}
		exitCode, err := svc.Execute(context.Background(), ctx.Args, ctx.Env)
		if exitCode != 0 && credential.IsVaultMode() && len(vaultTools) > 0 {
			markCredentialsStale(name, vaultTools)
		}
		return exitCode, err
	}

	// 5. Resolve the real binary path
	binaryPath, err := resolveBinary(def)
	if err != nil {
		return 1, fmt.Errorf("cannot find %q binary: %w", name, err)
	}

	// 6. Run before hooks
	if err := middleware.RunBefore(def.Before, ctx); err != nil {
		return 1, err
	}

	// 7-8. Execute binary
	// If no after hooks, passthrough stdin/stdout/stderr directly (streaming)
	if len(def.After) == 0 {
		exitCode, err := executePassthrough(binaryPath, ctx.Args, ctx.Env)
		// 9. On non-zero exit in vault mode, mark credentials stale
		if exitCode != 0 && credential.IsVaultMode() && len(vaultTools) > 0 {
			markCredentialsStale(name, vaultTools)
		}
		return exitCode, err
	}

	// With after hooks, capture output for processing
	ctx.ExitCode, ctx.Stdout, ctx.Stderr, err = executeBuffered(binaryPath, ctx.Args, ctx.Env)
	if err != nil && ctx.ExitCode == 0 {
		return 1, err
	}

	// 8. Run after hooks
	if err := middleware.RunAfter(def.After, ctx); err != nil {
		return ctx.ExitCode, err
	}

	os.Stdout.Write(ctx.Stdout)
	os.Stderr.Write(ctx.Stderr)

	// 9. On non-zero exit in vault mode, mark credentials stale
	if ctx.ExitCode != 0 && credential.IsVaultMode() && len(vaultTools) > 0 {
		markCredentialsStale(name, vaultTools)
	}

	return ctx.ExitCode, nil
}

// uniqueVaultTools returns deduplicated vault_tool values from credential bindings.
func uniqueVaultTools(bindings []registry.CredentialBinding) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, b := range bindings {
		if b.Source.VaultTool != "" {
			if _, ok := seen[b.Source.VaultTool]; !ok {
				seen[b.Source.VaultTool] = struct{}{}
				result = append(result, b.Source.VaultTool)
			}
		}
	}
	return result
}

// markCredentialsStale marks cached vault credentials as stale and prints a hint to stderr.
func markCredentialsStale(toolName string, vaultTools []string) {
	workspaceID := os.Getenv("ANYCLI_VAULT_WORKSPACE_ID")
	if workspaceID == "" {
		return
	}
	for _, vt := range vaultTools {
		_ = credential.MarkStale(workspaceID, vt)
	}
	fmt.Fprintf(os.Stderr, "[anycli] credentials for %q may be stale. retry the same command to fetch fresh credentials.\n", toolName)
}

// resolveBinary finds the real binary path, skipping the anycli shim.
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
