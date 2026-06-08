// Package anycli is the embeddable core for "run an underlying CLI/API tool
// with injected credentials + middleware". A host (e.g. heliox) constructs an
// Engine, provides a CredentialResolver, and calls Engine.Execute; AnyCLI loads
// the embedded tool definition, resolves credentials through the resolver,
// injects them (env / arg / file), runs before/after middleware, and execs the
// underlying binary or built-in service.
//
// Tool definitions are internal to AnyCLI (embedded JSON under definitions/);
// the consumer never supplies them. The consumer supplies only a
// CredentialResolver and, optionally, a Cache.
//
// See docs/design/002-embeddable-core-and-credential-resolver.md.
package anycli

import (
	"context"

	"github.com/shipbase/anycli/internal/credential"
	"github.com/shipbase/anycli/internal/exec"
)

// Tool identifies a tool by its definition name. It is a named type for
// type-safety + discoverability. AnyCLI ships no tool-name constants yet — the
// supported-tool definitions are added internally in a later round. Pass a raw
// Tool("…") whose name matches an embedded definition; validity is checked at
// runtime against the embedded definition set, so an unknown tool is an error
// from Execute, not a compile error.
type Tool = credential.Tool

// Credential holds the in-memory credential data a resolver returns for a tool.
// It is the only thing that crosses the resolver boundary into AnyCLI.
type Credential = credential.Credential

// CredentialResolver is the seam through which a host supplies credentials.
// The resolver returns in-memory data only; AnyCLI owns injection, caching, and
// lifecycle. The resolver never learns how the data is injected.
type CredentialResolver = credential.CredentialResolver

// Cache is the credential cache the engine uses to avoid re-resolving on every
// call. It is consumer-supplied so a host can back it with a per-process /
// per-assistant in-memory store instead of any on-disk cache. The cache stores
// entries keyed by tool name; the engine interprets freshness (CacheUntil /
// Stale), the implementation only stores and retrieves.
type Cache = credential.Cache

// CacheEntry is one cached credential: the extracted fields plus the freshness
// metadata the engine uses to decide whether to re-resolve.
type CacheEntry = credential.CacheEntry

// Config carries the consumer-supplied initialization for an Engine. It carries
// only a Cache — tool definitions are internal to AnyCLI (embedded) and are
// never consumer-supplied.
type Config struct {
	// Cache is the credential cache the engine uses. Optional: a nil Cache
	// installs an in-memory default (see NewMemoryCache).
	Cache Cache
}

// NewMemoryCache returns an empty in-memory Cache — the default the engine
// installs when Config.Cache is nil. Exposed so a consumer can construct one
// explicitly (e.g. one per assistant).
func NewMemoryCache() Cache {
	return credential.NewMemoryCache()
}

// Engine is the embeddable AnyCLI core. Construct it with New, then call
// Execute. It is safe for concurrent use to the extent its Cache is.
type Engine struct {
	inner *exec.Engine
}

// New constructs an Engine from cfg. A nil cfg.Cache installs the in-memory
// default cache.
func New(cfg Config) (*Engine, error) {
	cache := cfg.Cache
	if cache == nil {
		cache = credential.NewMemoryCache()
	}
	inner, err := exec.NewEngine(cache)
	if err != nil {
		return nil, err
	}
	return &Engine{inner: inner}, nil
}

// Execute loads the embedded definition for tool, resolves and injects its
// credentials via resolver, runs middleware, and execs the underlying binary or
// built-in service.
//
// resolver must be non-nil; an unknown tool (no embedded definition) returns an
// error.
func (e *Engine) Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (exitCode int, err error) {
	return e.inner.Execute(ctx, string(tool), args, resolver)
}
