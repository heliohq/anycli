package credential

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// VaultResponse represents the vault effective endpoint response.
type VaultResponse struct {
	Credentials []VaultCredential `json:"credentials"`
}

// VaultCredential represents a single credential entry from the vault.
type VaultCredential struct {
	ID         string                 `json:"id"`
	Tool       string                 `json:"tool"`
	Source     string                 `json:"source"`
	Type       string                 `json:"type"`
	Data       map[string]interface{} `json:"data"`
	Metadata   map[string]interface{} `json:"metadata"`
	Status     string                 `json:"status"`
	CacheUntil *string                `json:"cache_until,omitempty"`
}

// VaultFetchError indicates a transient vault failure where stale cache may be used.
type VaultFetchError struct {
	Err error
}

func (e *VaultFetchError) Error() string {
	return fmt.Sprintf("vault fetch error: %v", e.Err)
}

func (e *VaultFetchError) Unwrap() error {
	return e.Err
}

// FetchFromVault calls the vault effective endpoint for a specific tool.
// Returns the credential data and cache_until time, or an error.
// Timeout is 5 seconds. No retries.
// On 4xx: returns error immediately (no cache fallback).
// On 5xx or network error: returns a VaultFetchError so caller can decide to use stale cache.
func FetchFromVault(cfg *VaultConfig, vaultTool string) (*VaultCredential, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	endpoint := fmt.Sprintf(
		"%s/vault/credentials/effective?workspace_id=%s&tool=%s",
		cfg.URL,
		url.QueryEscape(cfg.WorkspaceID),
		url.QueryEscape(vaultTool),
	)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &VaultFetchError{Err: fmt.Errorf("failed to create request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, &VaultFetchError{Err: fmt.Errorf("request failed: %w", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &VaultFetchError{Err: fmt.Errorf("failed to read response body: %w", err)}
	}

	// 4xx errors are permanent failures (e.g., auth denied, not found)
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body))
	}

	// 5xx errors are transient; caller may fall back to stale cache
	if resp.StatusCode >= 500 {
		return nil, &VaultFetchError{
			Err: fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body)),
		}
	}

	var vaultResp VaultResponse
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, &VaultFetchError{
			Err: fmt.Errorf("failed to parse vault response: %w", err),
		}
	}

	if len(vaultResp.Credentials) == 0 {
		return nil, fmt.Errorf("vault returned no credentials for tool %q", vaultTool)
	}

	return &vaultResp.Credentials[0], nil
}
