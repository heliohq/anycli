package credential

import (
	"sync"
	"time"
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

// CacheKey derives the cache map key for one (tool, account). NUL cannot
// appear in either part, so the join is collision-free: distinct accounts of
// the same tool never share an entry, and the default account ("") keys
// distinctly from any named account (design 003).
func CacheKey(tool, account string) string { return tool + "\x00" + account }

// Cache is the credential cache the engine uses to avoid re-resolving on every
// call. It is a consumer-supplied interface: a host can back it with a
// per-process / per-assistant in-memory map, a shared store, or anything else.
// The engine never assumes on-disk storage. The cache stores entries keyed by
// CacheKey(tool, account) — the engine derives the key; freshness
// (CacheUntil / Stale) is interpreted by the engine, not by the Cache
// implementation — the implementation only stores and retrieves.
type Cache interface {
	// Get returns the cached entry for a key and whether one exists.
	Get(key string) (*CacheEntry, bool)
	// Set stores (or replaces) the cached entry for a key.
	Set(key string, entry *CacheEntry)
	// MarkStale marks an existing entry stale so the next resolve refetches.
	// A no-op if no entry exists for the key.
	MarkStale(key string)
}

// memCache is the default in-memory Cache used when Config.Cache is nil. It is
// safe for concurrent use.
type memCache struct {
	mu      sync.Mutex
	entries map[string]*CacheEntry
}

// NewMemoryCache returns an empty in-memory Cache. This is the default the
// engine installs when the consumer does not supply one.
func NewMemoryCache() Cache {
	return &memCache{entries: make(map[string]*CacheEntry)}
}

func (c *memCache) Get(key string) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	return entry, ok
}

func (c *memCache) Set(key string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry
}

func (c *memCache) MarkStale(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		entry.Stale = true
	}
}
