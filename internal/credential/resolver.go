package credential

import (
	"context"
	"time"
)

// Tool identifies a tool by its definition name. It is a named type for
// type-safety + discoverability. Validity is checked at runtime against the
// embedded definition set, not at compile time.
type Tool string

// Credential holds the in-memory credential data a resolver returns for a tool.
// It is the only thing that crosses the resolver boundary into AnyCLI.
type Credential struct {
	// Data holds the tool's credential fields. Bindings index into it by the
	// definition's VaultField (e.g. Data["access_token"]). Its shape is the
	// resolver's choice.
	Data map[string]any
	// CacheUntil is when this credential goes stale. The resolver is the only
	// party that knows it (a stored token's expiry, a minted token's TTL, ...).
	// AnyCLI uses it to manage its cache; AnyCLI does not decide it.
	CacheUntil time.Time
}

// CredentialResolver is the seam through which a host supplies credentials.
// The resolver returns in-memory data only; AnyCLI owns injection, caching,
// and lifecycle. The resolver never learns how the data is injected.
type CredentialResolver interface {
	// Resolve returns the credential fields for one tool, plus when they go stale.
	Resolve(ctx context.Context, tool Tool) (*Credential, error)
}
