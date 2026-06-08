// Package anycli is the embeddable core for "run an underlying CLI/API tool
// with injected credentials + middleware". A host (e.g. heliox) provides a
// CredentialResolver and calls Execute; AnyCLI loads the embedded tool
// definition, resolves credentials through the resolver, injects them
// (env / arg / file), runs before/after middleware, and execs the underlying
// binary or built-in service.
//
// See docs/design/002-embeddable-core-and-credential-resolver.md.
package anycli

import (
	"context"

	"github.com/shipbase/anycli/internal/credential"
	"github.com/shipbase/anycli/internal/exec"
)

// Tool identifies a tool by its definition name. It is a named type (not a
// bare string) for type-safety + discoverability — AnyCLI ships one constant
// per embedded definition. It is NOT a closed compile-time set: validity is
// checked at runtime against the embedded definitions, so adding a tool stays
// "drop in a JSON definition", optionally plus a constant. Callers may pass a
// constant (ToolGitHub) or a raw Tool("…") for a definition AnyCLI doesn't ship
// a constant for; an unknown tool is an error from Execute, not a compile error.
type Tool = credential.Tool

const (
	// ToolGitHub is the GitHub CLI (gh) tool definition.
	ToolGitHub Tool = "gh"
	// ToolWrangler is the Cloudflare Wrangler CLI tool definition.
	ToolWrangler Tool = "wrangler"
)

// Credential holds the in-memory credential data a resolver returns for a tool.
// It is the only thing that crosses the resolver boundary into AnyCLI.
type Credential = credential.Credential

// CredentialResolver is the seam through which a host supplies credentials.
// The resolver returns in-memory data only; AnyCLI owns injection, caching, and
// lifecycle. The resolver never learns how the data is injected.
type CredentialResolver = credential.CredentialResolver

// Execute is the embeddable entrypoint. It loads the embedded definition for
// tool, resolves and injects its credentials via resolver, runs middleware, and
// execs the underlying binary or built-in service.
//
// resolver must be non-nil; an unknown tool (no embedded definition) returns an
// error.
func Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (exitCode int, err error) {
	return exec.Execute(ctx, string(tool), args, resolver)
}
