package credential

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/credential/format"
	"github.com/shipbase/anycli/internal/registry"
)

// InjectionResult holds the results of applying credential bindings.
type InjectionResult struct {
	Env     map[string]string // Environment variables to set
	Args    []string          // Arguments to prepend
	Cleanup func()            // Cleanup function for temp files (vault mode file inject)
}

// ApplyBindings applies resolved credentials according to their injection config.
// bindings and values must be parallel slices.
// isVaultMode indicates whether to use vault-mode file isolation.
func ApplyBindings(toolName string, bindings []registry.CredentialBinding, values []string, isVaultMode bool) (*InjectionResult, error) {
	if len(bindings) != len(values) {
		return nil, fmt.Errorf("bindings and values length mismatch: %d vs %d", len(bindings), len(values))
	}

	result := &InjectionResult{
		Env:  make(map[string]string),
		Args: nil,
	}

	var cleanups []func()

	for i, b := range bindings {
		val := values[i]
		if val == "" {
			// Skip unresolved credentials
			continue
		}

		switch b.Inject.Type {
		case "env":
			if b.Inject.EnvVar == "" {
				return nil, fmt.Errorf("binding %d: inject type 'env' requires env_var", i)
			}
			result.Env[b.Inject.EnvVar] = val

		case "arg":
			if b.Inject.Flag == "" {
				return nil, fmt.Errorf("binding %d: inject type 'arg' requires flag", i)
			}
			if b.Inject.Format == "eq" {
				result.Args = append(result.Args, b.Inject.Flag+"="+val)
			} else {
				result.Args = append(result.Args, b.Inject.Flag, val)
			}

		case "file":
			cleanup, err := applyFileBinding(toolName, i, b, val, isVaultMode, result)
			if err != nil {
				// Clean up anything we already created before returning
				for _, c := range cleanups {
					c()
				}
				return nil, err
			}
			if cleanup != nil {
				cleanups = append(cleanups, cleanup)
			}

		default:
			return nil, fmt.Errorf("binding %d: unknown inject type %q", i, b.Inject.Type)
		}
	}

	if len(cleanups) > 0 {
		result.Cleanup = func() {
			for _, c := range cleanups {
				c()
			}
		}
	}

	return result, nil
}

// applyFileBinding handles inject type "file".
// In standalone mode: patches the file at the configured path directly.
// In vault mode: creates a temp file, patches it, and redirects via config_env/config_flag.
func applyFileBinding(toolName string, index int, b registry.CredentialBinding, value string, isVaultMode bool, result *InjectionResult) (func(), error) {
	targetPath := expandPath(b.Inject.Path)
	if targetPath == "" {
		return nil, fmt.Errorf("binding %d: inject type 'file' requires path", index)
	}

	if !isVaultMode {
		// Standalone mode: patch the file at path directly
		if err := patchFile(targetPath, b.Inject, value); err != nil {
			return nil, fmt.Errorf("binding %d: failed to patch file %q: %w", index, targetPath, err)
		}
		return nil, nil
	}

	// Vault mode: create temp file for isolation
	if b.Inject.ConfigEnv == "" && b.Inject.ConfigFlag == "" {
		return nil, fmt.Errorf(
			"binding %d: vault mode file inject requires config_env or config_flag to redirect config path",
			index,
		)
	}

	// Create temp directory for this tool
	tmpDir := filepath.Join(config.TmpDir(), toolName)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("binding %d: failed to create temp dir: %w", index, err)
	}

	// Create a temp file with a predictable name based on binding index
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("cred_%d_%s", index, filepath.Base(b.Inject.Path)))

	// Copy original file if it exists
	if err := copyFileIfExists(targetPath, tmpPath); err != nil {
		return nil, fmt.Errorf("binding %d: failed to copy original file: %w", index, err)
	}

	// Patch the temp file
	if err := patchFile(tmpPath, b.Inject, value); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("binding %d: failed to patch temp file: %w", index, err)
	}

	// Redirect via config_env or config_flag
	if b.Inject.ConfigEnv != "" {
		result.Env[b.Inject.ConfigEnv] = tmpPath
	}
	if b.Inject.ConfigFlag != "" {
		result.Args = append(result.Args, b.Inject.ConfigFlag, tmpPath)
	}

	// Return cleanup function
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	return cleanup, nil
}

// patchFile writes a credential value to a file using the configured format.
func patchFile(filePath string, inject registry.CredentialInject, value string) error {
	mode := os.FileMode(0600)
	if inject.Mode != "" {
		parsed, err := strconv.ParseUint(inject.Mode, 8, 32)
		if err == nil {
			mode = os.FileMode(parsed)
		}
	}

	// Resolve templates in fields
	resolvedFields := make(map[string]string, len(inject.Fields))
	for k, tmpl := range inject.Fields {
		resolvedFields[k] = resolveTemplate(tmpl, value)
	}

	fileFormat := inject.FileFormat
	if fileFormat == "" {
		fileFormat = "json" // default
	}

	if fileFormat == "custom" {
		// Custom patchers are handled separately by the caller
		return fmt.Errorf("custom format requires a registered patcher")
	}

	return format.PatchFile(filePath, fileFormat, resolvedFields, mode)
}

// resolveTemplate replaces {{.Value}} or {{value}} in a template string with the actual value.
func resolveTemplate(tmpl, value string) string {
	if tmpl == "" || tmpl == "{{.Value}}" || tmpl == "{{value}}" {
		return value
	}
	result := strings.ReplaceAll(tmpl, "{{.Value}}", value)
	result = strings.ReplaceAll(result, "{{value}}", value)
	return result
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// copyFileIfExists copies src to dst. If src doesn't exist, creates an empty file at dst.
func copyFileIfExists(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(dst, nil, 0600)
		}
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
