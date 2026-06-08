package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/shipbase/anycli/internal/config"
)

// CacheEntry represents a cached credential. It stores only the extracted
// string fields needed for injection, never the full resolver Data blob.
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
// The cache is keyed by tool name: ~/.anycli/cache/<tool>.json
func cachePath(tool string) string {
	return filepath.Join(config.CacheDir(), tool+".json")
}

// ReadCache reads the cache file for a tool.
// Returns nil, nil if the cache doesn't exist.
func ReadCache(tool string) (*CacheEntry, error) {
	data, err := os.ReadFile(cachePath(tool))
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

// WriteCache writes a cache entry for a tool.
func WriteCache(tool string, entry *CacheEntry) error {
	p := cachePath(tool)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// MarkStale marks an existing cache entry as stale so the next invocation
// re-resolves. Reads the current entry, sets Stale=true, writes back.
func MarkStale(tool string) error {
	entry, err := ReadCache(tool)
	if err != nil {
		return err
	}
	if entry == nil {
		// No cache to mark stale
		return nil
	}
	entry.Stale = true
	return WriteCache(tool, entry)
}
