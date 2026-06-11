package credential

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/registry"
)

// setupHome creates a temp ANYCLI_HOME and points the env at it. Shared by the
// credential package tests.
func setupHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ANYCLI_HOME", dir)
	return dir
}

// stubResolver is a CredentialResolver backed by a fixed credential.
type stubResolver struct {
	cred  *Credential
	err   error
	calls int
}

func (s *stubResolver) Resolve(ctx context.Context, tool Tool) (*Credential, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.cred, nil
}

func bindings(fields ...string) []registry.CredentialBinding {
	var bs []registry.CredentialBinding
	for _, f := range fields {
		bs = append(bs, registry.CredentialBinding{
			Source: registry.CredentialSource{VaultTool: "tool", VaultField: f},
			Inject: registry.CredentialInject{Type: "env", EnvVar: f},
		})
	}
	return bs
}

func TestResolveBindings_NilResolver(t *testing.T) {
	_, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", bindings("access_token"), nil)
	if err == nil {
		t.Fatal("expected error for nil resolver")
	}
}

func TestResolveBindings_NilCache(t *testing.T) {
	r := &stubResolver{cred: &Credential{Data: map[string]any{"access_token": "x"}}}
	_, err := ResolveBindings(context.Background(), nil, "gh", bindings("access_token"), r)
	if err == nil {
		t.Fatal("expected error for nil cache")
	}
}

func TestResolveBindings_EmptyBindings(t *testing.T) {
	r := &stubResolver{}
	values, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", nil, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values != nil {
		t.Errorf("values = %v, want nil", values)
	}
	if r.calls != 0 {
		t.Errorf("resolver called %d times for empty bindings, want 0", r.calls)
	}
}

func TestResolveBindings_ResolvesAndExtractsFields(t *testing.T) {
	cache := NewMemoryCache()
	r := &stubResolver{cred: &Credential{
		Data: map[string]any{
			"access_token":  "ghp_abc",
			"refresh_token": "ghr_secret",
		},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}

	values, err := ResolveBindings(context.Background(), cache, "gh", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 1 || values[0] != "ghp_abc" {
		t.Fatalf("values = %v, want [ghp_abc]", values)
	}

	// Only the required field should be cached; refresh_token must never be stored.
	cached, ok := cache.Get("gh")
	if !ok || cached == nil {
		t.Fatal("expected cache entry to be written")
	}
	if cached.Fields["access_token"] != "ghp_abc" {
		t.Errorf("cached access_token = %q, want ghp_abc", cached.Fields["access_token"])
	}
	if _, ok := cached.Fields["refresh_token"]; ok {
		t.Error("refresh_token must NOT be cached — unbound fields must not leak into the cache")
	}
}

func TestResolveBindings_UsesFreshCache(t *testing.T) {
	cache := NewMemoryCache()
	// Pre-seed a fresh cache with the required field.
	cache.Set("gh", &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Fields:     map[string]string{"access_token": "cached_tok"},
	})

	r := &stubResolver{cred: &Credential{Data: map[string]any{"access_token": "fresh_tok"}}}
	values, err := ResolveBindings(context.Background(), cache, "gh", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values[0] != "cached_tok" {
		t.Errorf("values[0] = %q, want cached_tok (cache should be used)", values[0])
	}
	if r.calls != 0 {
		t.Errorf("resolver called %d times when cache is fresh, want 0", r.calls)
	}
}

func TestResolveBindings_ReResolvesWhenCacheMissingField(t *testing.T) {
	cache := NewMemoryCache()
	// Fresh cache that lacks one of the required fields.
	cache.Set("aws", &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Fields:     map[string]string{"access_key_id": "AKIA"},
	})

	r := &stubResolver{cred: &Credential{
		Data: map[string]any{
			"access_key_id":     "AKIA-fresh",
			"secret_access_key": "secret-fresh",
		},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}

	values, err := ResolveBindings(context.Background(), cache, "aws", bindings("access_key_id", "secret_access_key"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (cache missing a required field)", r.calls)
	}
	if values[0] != "AKIA-fresh" || values[1] != "secret-fresh" {
		t.Errorf("values = %v, want [AKIA-fresh secret-fresh]", values)
	}
}

func TestResolveBindings_ReResolvesWhenStale(t *testing.T) {
	cache := NewMemoryCache()
	cache.Set("gh", &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      true,
		Fields:     map[string]string{"access_token": "stale_tok"},
	})

	r := &stubResolver{cred: &Credential{
		Data:       map[string]any{"access_token": "fresh_tok"},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}
	values, err := ResolveBindings(context.Background(), cache, "gh", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (cache stale)", r.calls)
	}
	if values[0] != "fresh_tok" {
		t.Errorf("values[0] = %q, want fresh_tok", values[0])
	}
}

func TestResolveBindings_NilCredentialSkips(t *testing.T) {
	r := &stubResolver{cred: nil}
	values, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 1 || values[0] != "" {
		t.Errorf("values = %v, want [\"\"] for nil credential", values)
	}
}

func TestResolveBindings_ResolverError(t *testing.T) {
	r := &stubResolver{err: errors.New("mint failed")}
	_, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", bindings("access_token"), r)
	if err == nil {
		t.Fatal("expected error from resolver to propagate")
	}
}

func TestResolveBindings_ZeroCacheUntilNotReused(t *testing.T) {
	cache := NewMemoryCache()
	r := &stubResolver{cred: &Credential{
		Data: map[string]any{"access_token": "tok"},
		// CacheUntil zero => ephemeral, not reused on next call.
	}}

	if _, err := ResolveBindings(context.Background(), cache, "gh", bindings("access_token"), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second call must re-resolve because the written entry is immediately expired.
	if _, err := ResolveBindings(context.Background(), cache, "gh", bindings("access_token"), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 2 {
		t.Errorf("resolver called %d times, want 2 (zero CacheUntil must not be reused)", r.calls)
	}
}

func TestExtractStringFields_ScalarsOnly(t *testing.T) {
	out := extractStringFields(map[string]any{
		"s": "str",
		"f": float64(42),
		"b": true,
		"o": map[string]any{"nested": "x"},
		"a": []any{"y"},
		"n": nil,
	})
	if out["s"] != "str" || out["f"] != "42" || out["b"] != "true" {
		t.Errorf("scalar extraction wrong: %v", out)
	}
	if _, ok := out["o"]; ok {
		t.Error("nested object must not be extracted")
	}
	if _, ok := out["a"]; ok {
		t.Error("array must not be extracted")
	}
	if _, ok := out["n"]; ok {
		t.Error("nil must not be extracted")
	}
}
