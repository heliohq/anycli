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
	"github.com/shipbase/anycli/internal/tools"
)

// InjectionResult holds the results of applying credential bindings.
type InjectionResult struct {
	Env     map[string]string // Environment variables to set
	Args    []string          // Arguments to append
	Cleanup func()            // Cleanup function for temp files (vault mode file inject)
}

// fileBindingEntry holds a file binding together with its resolved value and original index.
type fileBindingEntry struct {
	index   int
	binding registry.CredentialBinding
	value   string
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

	// First pass: collect file bindings grouped by target path; handle env/arg immediately.
	// Use a slice to maintain insertion order of groups.
	var fileGroupOrder []string
	fileGroups := make(map[string][]fileBindingEntry)

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
			targetPath := expandPath(b.Inject.Path)
			if targetPath == "" {
				return nil, fmt.Errorf("binding %d: inject type 'file' requires path", i)
			}
			if _, exists := fileGroups[targetPath]; !exists {
				fileGroupOrder = append(fileGroupOrder, targetPath)
			}
			fileGroups[targetPath] = append(fileGroups[targetPath], fileBindingEntry{
				index:   i,
				binding: b,
				value:   val,
			})

		default:
			return nil, fmt.Errorf("binding %d: unknown inject type %q", i, b.Inject.Type)
		}
	}

	// Second pass: process file binding groups.
	// In vault mode, create one unique temp directory per ApplyBindings call.
	var tmpDir string
	if isVaultMode && len(fileGroups) > 0 {
		parentDir := config.TmpDir()
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create temp parent dir: %w", err)
		}
		var err error
		tmpDir, err = os.MkdirTemp(parentDir, toolName+"-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		cleanups = append(cleanups, func() {
			_ = os.RemoveAll(tmpDir)
		})
	}

	for _, targetPath := range fileGroupOrder {
		entries := fileGroups[targetPath]
		cleanup, err := applyFileBindingGroup(tmpDir, targetPath, entries, isVaultMode, result)
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

// applyFileBindingGroup handles a group of file bindings that all target the same path.
// In standalone mode: patches the file at the configured path directly with all fields.
// In vault mode: creates ONE temp file per unique target path (inside tmpDir),
// copies the original, patches ALL fields from the group, and sets config_env/config_flag once.
func applyFileBindingGroup(tmpDir, targetPath string, entries []fileBindingEntry, isVaultMode bool, result *InjectionResult) (func(), error) {
	if !isVaultMode {
		// Standalone mode: patch the file at path directly for each binding
		var patchCleanups []func() error
		for _, e := range entries {
			patchCleanup, err := patchFile(targetPath, e.binding.Inject, e.value)
			if err != nil {
				return nil, fmt.Errorf("binding %d: failed to patch file %q: %w", e.index, targetPath, err)
			}
			if patchCleanup != nil {
				patchCleanups = append(patchCleanups, patchCleanup)
			}
		}
		if len(patchCleanups) > 0 {
			return func() {
				for _, c := range patchCleanups {
					_ = c()
				}
			}, nil
		}
		return nil, nil
	}

	// Vault mode: validate that at least one binding provides config_env or config_flag
	var configEnv, configFlag string
	for _, e := range entries {
		if e.binding.Inject.ConfigEnv == "" && e.binding.Inject.ConfigFlag == "" {
			return nil, fmt.Errorf(
				"binding %d: vault mode file inject requires config_env or config_flag to redirect config path",
				e.index,
			)
		}
		// Use the first non-empty config_env/config_flag from the group
		if configEnv == "" && e.binding.Inject.ConfigEnv != "" {
			configEnv = e.binding.Inject.ConfigEnv
		}
		if configFlag == "" && e.binding.Inject.ConfigFlag != "" {
			configFlag = e.binding.Inject.ConfigFlag
		}
	}

	// Create a uniquely-named temp file to avoid basename collisions
	// (e.g., /a/config.json and /b/config.json both have base "config.json")
	tmpFile, err := os.CreateTemp(tmpDir, filepath.Base(targetPath)+"-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for %q: %w", targetPath, err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Copy original file if it exists
	if err := copyFileIfExists(targetPath, tmpPath); err != nil {
		return nil, fmt.Errorf("failed to copy original file %q: %w", targetPath, err)
	}

	// Patch the temp file with ALL fields from all bindings in this group
	var patchCleanups []func() error
	for _, e := range entries {
		patchCleanup, err := patchFile(tmpPath, e.binding.Inject, e.value)
		if err != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("binding %d: failed to patch temp file: %w", e.index, err)
		}
		if patchCleanup != nil {
			patchCleanups = append(patchCleanups, patchCleanup)
		}
	}

	// Redirect via config_env or config_flag (once per unique target path)
	if configEnv != "" {
		result.Env[configEnv] = tmpPath
	}
	if configFlag != "" {
		result.Args = append(result.Args, configFlag, tmpPath)
	}

	if len(patchCleanups) > 0 {
		return func() {
			for _, c := range patchCleanups {
				_ = c()
			}
		}, nil
	}

	return nil, nil
}

// patchFile writes a credential value to a file using the configured format.
// Returns an optional cleanup function (non-nil only for custom patchers).
func patchFile(filePath string, inject registry.CredentialInject, value string) (func() error, error) {
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
		if inject.Patcher == "" {
			return nil, fmt.Errorf("custom format requires patcher name in definition")
		}
		patcher, err := tools.GetPatcher(inject.Patcher)
		if err != nil {
			return nil, err
		}
		cleanup, err := patcher.Patch(filePath, resolvedFields, mode)
		if err != nil {
			return nil, err
		}
		return cleanup, nil
	}

	return nil, format.PatchFile(filePath, fileFormat, resolvedFields, mode)
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
