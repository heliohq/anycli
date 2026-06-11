package credential

import (
	"context"
	"fmt"
	"time"

	"github.com/heliohq/anycli/internal/registry"
)

// ResolveBindings produces the per-binding credential values for one
// (tool, account) using the supplied resolver, parallel to the input bindings
// slice. An empty string means the credential field was not found (the binding
// is skipped on inject). account "" selects the resolver's default account.
//
// The cache is consulted first, keyed by CacheKey(tool, account): a fresh
// (non-stale, unexpired) cache entry that contains all required fields is used
// without calling the resolver. Otherwise the resolver is invoked, the
// extracted fields are written to the cache keyed by the resolver-supplied
// CacheUntil, and the fresh values are returned.
//
// cache must be non-nil; the engine always supplies one (the in-memory default
// when the consumer provides none).
func ResolveBindings(ctx context.Context, cache Cache, tool, account string, bindings []registry.CredentialBinding, resolver CredentialResolver) ([]string, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("credential resolver is nil")
	}
	if cache == nil {
		return nil, fmt.Errorf("credential cache is nil")
	}

	required := requiredFields(bindings)
	key := CacheKey(tool, account)

	// 1. Try the cache.
	if cached, ok := cache.Get(key); ok && cached != nil && cached.IsValid() && hasAllFields(cached.Fields, required) {
		return valuesForBindings(bindings, cached.Fields), nil
	}

	// 2. Resolve fresh credentials.
	cred, err := resolver.Resolve(ctx, Tool(tool), account)
	if err != nil {
		return nil, fmt.Errorf("resolve credentials for %q: %w", tool, err)
	}
	if cred == nil {
		// No credential for this tool — skip silently (tool may work without auth).
		return make([]string, len(bindings)), nil
	}

	// 3. Extract only the required fields from the resolver's Data.
	allFields := extractStringFields(cred.Data)
	fields := make(map[string]string, len(required))
	for f := range required {
		if v, ok := allFields[f]; ok {
			fields[f] = v
		}
	}

	// 4. Write to cache, keyed by the resolver-supplied CacheUntil.
	cacheUntil := cred.CacheUntil
	if cacheUntil.IsZero() {
		// No expiry hint from the resolver — do not cache across invocations.
		cacheUntil = time.Now()
	}
	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: cacheUntil,
		Stale:      false,
		Fields:     fields,
	}
	cache.Set(key, entry)

	return valuesForBindings(bindings, fields), nil
}

// requiredFields returns the set of vault_field values the bindings index into.
func requiredFields(bindings []registry.CredentialBinding) map[string]struct{} {
	required := make(map[string]struct{})
	for _, b := range bindings {
		if b.Source.VaultField != "" {
			required[b.Source.VaultField] = struct{}{}
		}
	}
	return required
}

// hasAllFields reports whether fields contains every key in required.
func hasAllFields(fields map[string]string, required map[string]struct{}) bool {
	for f := range required {
		if _, ok := fields[f]; !ok {
			return false
		}
	}
	return true
}

// valuesForBindings maps each binding to its field value (empty if absent),
// producing a slice parallel to bindings.
func valuesForBindings(bindings []registry.CredentialBinding, fields map[string]string) []string {
	values := make([]string, len(bindings))
	for i, b := range bindings {
		if b.Source.VaultField != "" {
			values[i] = fields[b.Source.VaultField]
		}
	}
	return values
}

// extractStringFields converts the resolver's Data map to a flat string map,
// keeping only scalar values. Complex types (nested objects, arrays, nil) are
// skipped because bindings index into scalar credential fields.
func extractStringFields(data map[string]any) map[string]string {
	fields := make(map[string]string, len(data))
	for k, v := range data {
		switch val := v.(type) {
		case string:
			fields[k] = val
		case float64:
			fields[k] = fmt.Sprintf("%v", val)
		case bool:
			fields[k] = fmt.Sprintf("%v", val)
		}
	}
	return fields
}
