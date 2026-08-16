// Package anycli is the embeddable core for "run an underlying CLI/API tool
// with injected credentials + middleware". A host constructs an
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
	"fmt"
	"net/http"

	"github.com/heliohq/anycli/internal/credential"
	"github.com/heliohq/anycli/internal/exec"
	"github.com/heliohq/anycli/internal/tools/execution"
)

// Tool identifies a tool by its definition name. It is a named type for
// type-safety + discoverability. Tool names come from the embedded definition
// set returned by ListTools; AnyCLI intentionally does not duplicate that set
// as exported constants. Pass a raw Tool("…") whose name matches an embedded
// definition. An unknown tool is an error from Execute, not a compile error.
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
// entries keyed by (tool, account); the engine interprets freshness
// (CacheUntil / Stale), while the implementation only stores and retrieves.
type Cache = credential.Cache

// CacheEntry is one cached credential: the extracted fields plus the freshness
// metadata the engine uses to decide whether to re-resolve.
type CacheEntry = credential.CacheEntry

// Config carries the consumer-supplied initialization for an Engine. Tool
// definitions are internal to AnyCLI (embedded) and are never
// consumer-supplied.
type Config struct {
	// Cache is the credential cache the engine uses. Optional: a nil Cache
	// installs an in-memory default (see NewMemoryCache).
	Cache Cache

	// HTTPClient, when non-nil, is used for every outbound HTTP request the
	// engine makes: built-in service provider calls and lazy binary
	// downloads alike. nil keeps the default behavior (each call site's
	// production client, typically http.DefaultClient). The seam exists so
	// a host can interpose a RoundTripper — e.g. an e2e harness rewriting
	// provider upstreams to a local fixture server.
	HTTPClient *http.Client

	// FS is where every local file a tool reads or writes goes. Optional: a nil
	// FS installs the machine the process runs on (see execution.OS), which is
	// the right implementation for a person at a terminal. A host running
	// AnyCLI on shared infrastructure has a different machine underneath, and
	// this is how it decides what "local" means there.
	//
	// The seam governs the files a tool opens itself. A tool that hands a path
	// to a subprocess — mongodb, and the passthrough CLI tools — is outside it
	// by construction; a host that cares about isolation declines to run those
	// rather than assume this covers them.
	FS FileSystem
}

// FileSystem is the seam Config.FS is set to. See execution.FileSystem.
type FileSystem = execution.FileSystem

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
	if _, err := ListTools(); err != nil {
		return nil, fmt.Errorf("validate bundled tools: %w", err)
	}
	cache := cfg.Cache
	if cache == nil {
		cache = credential.NewMemoryCache()
	}
	inner, err := exec.NewEngine(cache, cfg.HTTPClient, cfg.FS)
	if err != nil {
		return nil, err
	}
	return &Engine{inner: inner}, nil
}

// ExecOptions carries per-invocation execution options (design 003).
type ExecOptions struct {
	// Account selects which connected account's credential to resolve when
	// the host has several for one tool. Empty = the resolver's default.
	Account string
}

// ExecuteWith loads the embedded definition for tool, resolves and injects its
// credentials via resolver for the account selected by opts, runs middleware,
// and execs the underlying binary or built-in service.
//
// resolver must be non-nil; an unknown tool (no embedded definition) returns an
// error.
func (e *Engine) ExecuteWith(ctx context.Context, tool Tool, args []string, resolver CredentialResolver, opts ExecOptions) (exitCode int, err error) {
	return e.inner.Execute(ctx, string(tool), args, resolver, opts.Account)
}

// Execute runs with the default account. Kept as the short form of ExecuteWith.
func (e *Engine) Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (exitCode int, err error) {
	return e.ExecuteWith(ctx, tool, args, resolver, ExecOptions{})
}
