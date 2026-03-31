package credential

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/shipbase/anycli/internal/registry"
)

const (
	envVaultURL         = "ANYCLI_VAULT_URL"
	envVaultToken       = "ANYCLI_VAULT_TOKEN"
	envVaultWorkspaceID = "ANYCLI_VAULT_WORKSPACE_ID"
)

// VaultConfig holds vault mode configuration from environment variables.
type VaultConfig struct {
	URL         string
	Token       string
	WorkspaceID string
}

// GetVaultConfig reads vault mode env vars. Returns nil if none are set.
// Returns error if partially configured (some but not all vars set).
func GetVaultConfig() (*VaultConfig, error) {
	vaultURL := os.Getenv(envVaultURL)
	vaultToken := os.Getenv(envVaultToken)
	workspaceID := os.Getenv(envVaultWorkspaceID)

	set := 0
	if vaultURL != "" {
		set++
	}
	if vaultToken != "" {
		set++
	}
	if workspaceID != "" {
		set++
	}

	if set == 0 {
		return nil, nil
	}
	if set < 3 {
		var missing []string
		if vaultURL == "" {
			missing = append(missing, envVaultURL)
		}
		if vaultToken == "" {
			missing = append(missing, envVaultToken)
		}
		if workspaceID == "" {
			missing = append(missing, envVaultWorkspaceID)
		}
		return nil, fmt.Errorf("vault mode partially configured, missing: %v", missing)
	}

	return &VaultConfig{
		URL:         vaultURL,
		Token:       vaultToken,
		WorkspaceID: workspaceID,
	}, nil
}

// IsVaultMode returns true if all vault env vars are set.
func IsVaultMode() bool {
	return os.Getenv(envVaultURL) != "" &&
		os.Getenv(envVaultToken) != "" &&
		os.Getenv(envVaultWorkspaceID) != ""
}

// Resolve fetches credentials for the given tool based on its credential bindings.
// Returns a slice of resolved values parallel to the input bindings slice.
// An empty string means the credential was not found (skipped silently).
//
// In vault mode: groups bindings by vault_tool, checks cache then fetches from vault,
// extracts vault_field values. On transient vault errors, falls back to stale cache.
// In standalone mode: reads from local credential file using local_key.
func Resolve(toolName string, bindings []registry.CredentialBinding) ([]string, error) {
	if len(bindings) == 0 {
		return nil, nil
	}

	cfg, err := GetVaultConfig()
	if err != nil {
		return nil, err
	}

	if cfg != nil {
		return resolveVault(cfg, bindings)
	}
	return resolveLocal(toolName, bindings)
}

// resolveLocal reads credentials from the local credential file.
func resolveLocal(toolName string, bindings []registry.CredentialBinding) ([]string, error) {
	creds, err := LoadLocal(toolName)
	if err != nil {
		return nil, fmt.Errorf("failed to load local credentials for %q: %w", toolName, err)
	}

	values := make([]string, len(bindings))
	if creds == nil {
		return values, nil
	}

	for i, b := range bindings {
		if b.Source.LocalKey != "" {
			values[i] = creds[b.Source.LocalKey]
		}
	}
	return values, nil
}

// resolveVault fetches credentials from the vault, using cache when available.
// Groups bindings by vault_tool to avoid duplicate fetches.
func resolveVault(cfg *VaultConfig, bindings []registry.CredentialBinding) ([]string, error) {
	values := make([]string, len(bindings))

	// Group bindings by vault_tool to avoid duplicate fetches.
	// For each unique vault_tool, we resolve once and extract fields.
	type resolvedTool struct {
		fields map[string]string
		err    error
	}
	cache := make(map[string]*resolvedTool)

	for i, b := range bindings {
		vaultTool := b.Source.VaultTool
		if vaultTool == "" {
			continue
		}

		rt, ok := cache[vaultTool]
		if !ok {
			fields, err := fetchVaultToolFields(cfg, vaultTool)
			rt = &resolvedTool{fields: fields, err: err}
			cache[vaultTool] = rt
		}

		if rt.err != nil {
			// Propagate non-transient errors; transient ones already attempted cache fallback.
			return nil, rt.err
		}

		if rt.fields != nil && b.Source.VaultField != "" {
			values[i] = rt.fields[b.Source.VaultField]
		}
	}

	return values, nil
}

// fetchVaultToolFields fetches credential fields for a vault_tool.
// Implements: check cache -> fetch from vault -> on transient error, fall back to stale cache.
func fetchVaultToolFields(cfg *VaultConfig, vaultTool string) (map[string]string, error) {
	// 1. Check cache first
	cached, err := ReadCache(cfg.WorkspaceID, vaultTool)
	if err != nil {
		// Cache read error is non-fatal; proceed to fetch
		cached = nil
	}

	if cached != nil && cached.IsValid() {
		return cached.Fields, nil
	}

	// 2. Fetch from vault
	cred, fetchErr := FetchFromVault(cfg, vaultTool)
	if fetchErr != nil {
		var vfe *VaultFetchError
		if errors.As(fetchErr, &vfe) {
			// Transient error: mark cache as stale and use it if available
			_ = MarkStale(cfg.WorkspaceID, vaultTool)
			if cached != nil && cached.Fields != nil {
				return cached.Fields, nil
			}
		}
		return nil, fetchErr
	}

	// 3. Extract string fields from credential data
	fields := extractStringFields(cred.Data)

	// 4. Determine cache expiration
	cacheUntil := time.Now().Add(5 * time.Minute) // default 5 minute cache
	if cred.CacheUntil != nil {
		if parsed, err := time.Parse(time.RFC3339, *cred.CacheUntil); err == nil {
			cacheUntil = parsed
		}
	}

	// 5. Write to cache
	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: cacheUntil,
		Stale:      false,
		Fields:     fields,
	}
	// Cache write error is non-fatal
	_ = WriteCache(cfg.WorkspaceID, vaultTool, entry)

	return fields, nil
}

// extractStringFields converts a map[string]interface{} to map[string]string,
// keeping only string values and converting others to their string representation.
func extractStringFields(data map[string]interface{}) map[string]string {
	fields := make(map[string]string, len(data))
	for k, v := range data {
		switch val := v.(type) {
		case string:
			fields[k] = val
		case float64:
			fields[k] = fmt.Sprintf("%v", val)
		case bool:
			fields[k] = fmt.Sprintf("%v", val)
		default:
			// Skip complex types (nested objects, arrays, nil)
		}
	}
	return fields
}
