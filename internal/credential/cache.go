package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/shipbase/anycli/internal/config"
)

// CacheEntry represents a cached credential.
type CacheEntry struct {
	FetchedAt  time.Time         `json:"fetched_at"`
	CacheUntil time.Time         `json:"cache_until"`
	Stale      bool              `json:"stale,omitempty"`
	Fields     map[string]string `json:"fields"`
}

// IsValid returns true if the cache entry is fresh (not stale and not expired).
func (e *CacheEntry) IsValid() bool {
	if e.Stale {
		return false
	}
	return time.Now().Before(e.CacheUntil)
}

// cachePath returns the file path for a cached credential entry.
func cachePath(workspaceID, vaultTool string) string {
	return filepath.Join(config.CacheDir(), workspaceID, vaultTool+".json")
}

// ReadCache reads the cache file for a tool in a workspace.
// Path: ~/.anycli/cache/<workspace_id>/<vault_tool>.json
// Returns nil, nil if cache doesn't exist.
func ReadCache(workspaceID, vaultTool string) (*CacheEntry, error) {
	data, err := os.ReadFile(cachePath(workspaceID, vaultTool))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// WriteCache writes a cache entry.
func WriteCache(workspaceID, vaultTool string, entry *CacheEntry) error {
	p := cachePath(workspaceID, vaultTool)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// MarkStale marks an existing cache entry as stale.
// Reads the current entry, sets Stale=true, writes back.
func MarkStale(workspaceID, vaultTool string) error {
	entry, err := ReadCache(workspaceID, vaultTool)
	if err != nil {
		return err
	}
	if entry == nil {
		// No cache to mark stale
		return nil
	}
	entry.Stale = true
	return WriteCache(workspaceID, vaultTool, entry)
}
