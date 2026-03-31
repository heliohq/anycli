package credential

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shipbase/anycli/internal/config"
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
	// Determine file permissions
	mode := os.FileMode(0600)
	if inject.Mode != "" {
		parsed, err := strconv.ParseUint(inject.Mode, 8, 32)
		if err == nil {
			mode = os.FileMode(parsed)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	switch inject.FileFormat {
	case "json":
		return patchJSON(filePath, inject.Fields, value, mode)
	case "ini":
		return patchINI(filePath, inject.Fields, value, mode)
	case "":
		// No format specified: write value directly
		return os.WriteFile(filePath, []byte(value), mode)
	default:
		// For yaml, toml, custom, etc. - write value directly as a basic fallback.
		// Full format support can be added as needed.
		return os.WriteFile(filePath, []byte(value), mode)
	}
}

// patchJSON patches a JSON file by setting fields to the credential value.
// Fields maps dot-paths to templates where {{value}} is replaced with the credential.
func patchJSON(filePath string, fields map[string]string, value string, mode os.FileMode) error {
	// Read existing file or start with empty object
	var data map[string]interface{}
	existing, err := os.ReadFile(filePath)
	if err == nil && len(existing) > 0 {
		if jsonErr := json.Unmarshal(existing, &data); jsonErr != nil {
			data = make(map[string]interface{})
		}
	} else {
		data = make(map[string]interface{})
	}

	// Apply fields
	for key, tmpl := range fields {
		resolved := resolveTemplate(tmpl, value)
		setNestedField(data, key, resolved)
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(filePath, out, mode)
}

// patchINI writes fields in INI format.
// Fields maps "section.key" to templates.
func patchINI(filePath string, fields map[string]string, value string, mode os.FileMode) error {
	// Read existing content or start fresh
	existing, _ := os.ReadFile(filePath)
	content := string(existing)

	for key, tmpl := range fields {
		resolved := resolveTemplate(tmpl, value)
		content = setINIValue(content, key, resolved)
	}

	return os.WriteFile(filePath, []byte(content), mode)
}

// resolveTemplate replaces {{value}} in a template string with the actual value.
func resolveTemplate(tmpl, value string) string {
	if tmpl == "" || tmpl == "{{value}}" {
		return value
	}
	return strings.ReplaceAll(tmpl, "{{value}}", value)
}

// setNestedField sets a value in a nested map using a dot-separated key path.
// For example, "oauth.access_token" would set data["oauth"]["access_token"].
func setNestedField(data map[string]interface{}, dotPath string, value string) {
	parts := strings.Split(dotPath, ".")
	if len(parts) == 0 {
		return
	}

	current := data
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			nextMap := make(map[string]interface{})
			current[parts[i]] = nextMap
			current = nextMap
			continue
		}
		if nextMap, ok := next.(map[string]interface{}); ok {
			current = nextMap
		} else {
			// Overwrite non-map value with a new map
			nextMap := make(map[string]interface{})
			current[parts[i]] = nextMap
			current = nextMap
		}
	}
	current[parts[len(parts)-1]] = value
}

// setINIValue sets or updates a key in INI content.
// key format: "section.key" or just "key" for global section.
func setINIValue(content, key, value string) string {
	dotIdx := strings.Index(key, ".")
	if dotIdx < 0 {
		// Global key (no section)
		line := key + " = " + value + "\n"
		return appendOrReplaceLine(content, "", key, line)
	}

	section := key[:dotIdx]
	fieldKey := key[dotIdx+1:]
	line := fieldKey + " = " + value + "\n"
	return appendOrReplaceLine(content, section, fieldKey, line)
}

// appendOrReplaceLine replaces an existing key line or appends it in the correct INI section.
func appendOrReplaceLine(content, section, key, newLine string) string {
	lines := strings.SplitAfter(content, "\n")
	// Remove trailing empty element from SplitAfter if content ends with \n
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	inSection := section == ""
	replaced := false
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a section header
		if len(trimmed) > 0 && trimmed[0] == '[' {
			if section != "" && !replaced && inSection {
				// We were in the right section but didn't find the key; insert before next section
				result = append(result, newLine)
				replaced = true
			}
			sectionName := extractSectionName(trimmed)
			inSection = sectionName == section
		}

		// Check if this line sets the key we're looking for
		if inSection && !replaced && isKeyLine(trimmed, key) {
			result = append(result, newLine)
			replaced = true
			continue
		}

		result = append(result, line)
	}

	if !replaced {
		if section != "" {
			// Check if section header already exists
			hasSectionHeader := false
			for _, line := range lines {
				if extractSectionName(strings.TrimSpace(line)) == section {
					hasSectionHeader = true
					break
				}
			}
			if !hasSectionHeader {
				result = append(result, "["+section+"]\n")
			}
			result = append(result, newLine)
		} else {
			result = append(result, newLine)
		}
	}

	return strings.Join(result, "")
}

// extractSectionName extracts the section name from an INI section header line like "[section]".
func extractSectionName(line string) string {
	if len(line) < 2 || line[0] != '[' {
		return ""
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return ""
	}
	return line[1:end]
}

// isKeyLine checks if an INI line sets the given key.
func isKeyLine(line, key string) bool {
	if !strings.HasPrefix(line, key) {
		return false
	}
	rest := strings.TrimSpace(line[len(key):])
	return len(rest) > 0 && rest[0] == '='
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
