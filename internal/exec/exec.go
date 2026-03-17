package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/middleware"
	"github.com/shipbase/anycli/internal/registry"
)

// Run executes a tool through the full middleware pipeline.
func Run(name string, args []string) (int, error) {
	def, err := registry.Load(name)
	if err != nil {
		return 1, err
	}

	binaryPath, err := resolveBinary(def)
	if err != nil {
		return 1, fmt.Errorf("cannot find %q binary: %w", name, err)
	}

	ctx := &middleware.Context{
		Args: args,
		Env:  make(map[string]string),
	}

	// Load credentials into ctx.Env if auth is configured
	if def.Auth != nil && def.Auth.EnvVar != "" {
		if cred, err := loadCredential(name, def.Auth.EnvVar); err == nil {
			ctx.Env[def.Auth.EnvVar] = cred
		}
	}

	// Run before hooks
	if err := middleware.RunBefore(def.Before, ctx); err != nil {
		return 1, err
	}

	// If no after hooks, passthrough stdin/stdout/stderr directly (streaming)
	if len(def.After) == 0 {
		return executePassthrough(binaryPath, ctx.Args, ctx.Env)
	}

	// With after hooks, capture output for processing
	ctx.ExitCode, ctx.Stdout, ctx.Stderr, err = executeBuffered(binaryPath, ctx.Args, ctx.Env)
	if err != nil && ctx.ExitCode == 0 {
		return 1, err
	}

	if err := middleware.RunAfter(def.After, ctx); err != nil {
		return ctx.ExitCode, err
	}

	os.Stdout.Write(ctx.Stdout)
	os.Stderr.Write(ctx.Stderr)

	return ctx.ExitCode, nil
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

// loadCredential reads a stored credential value for a tool.
func loadCredential(name, envVar string) (string, error) {
	path := filepath.Join(config.CredentialsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", err
	}

	val, ok := creds[envVar]
	if !ok {
		return "", fmt.Errorf("credential %s not found for %s", envVar, name)
	}
	return val, nil
}
