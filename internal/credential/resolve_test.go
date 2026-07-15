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

// stubResolver is a CredentialResolver backed by a fixed credential. It
// records the accounts it was asked to resolve.
type stubResolver struct {
	cred     *Credential
	err      error
	calls    int
	accounts []string
}

func (s *stubResolver) Resolve(ctx context.Context, tool Tool, account string) (*Credential, error) {
	s.calls++
	s.accounts = append(s.accounts, account)
	if s.err != nil {
		return nil, s.err
	}
	return s.cred, nil
}

func bindings(fields ...string) []registry.CredentialBinding {
	var bs []registry.CredentialBinding
	for _, f := range fields {
		bs = append(bs, registry.CredentialBinding{
			Source: registry.CredentialSource{Field: f},
			Inject: registry.CredentialInject{Type: "env", EnvVar: f},
		})
	}
	return bs
}

func TestResolveBindings_NilResolver(t *testing.T) {
	_, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", "", bindings("access_token"), nil)
	if err == nil {
		t.Fatal("expected error for nil resolver")
	}
}

func TestResolveBindings_NilCache(t *testing.T) {
	r := &stubResolver{cred: &Credential{Data: map[string]string{"access_token": "x"}}}
	_, err := ResolveBindings(context.Background(), nil, "gh", "", bindings("access_token"), r)
	if err == nil {
		t.Fatal("expected error for nil cache")
	}
}

func TestResolveBindings_EmptyBindings(t *testing.T) {
	r := &stubResolver{}
	values, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", "", nil, r)
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
		Data: map[string]string{
			"access_token":  "ghp_abc",
			"refresh_token": "ghr_secret",
		},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}

	values, err := ResolveBindings(context.Background(), cache, "gh", "", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 1 || values[0] != "ghp_abc" {
		t.Fatalf("values = %v, want [ghp_abc]", values)
	}

	// Only the required field should be cached; refresh_token must never be stored.
	cached, ok := cache.Get(CacheKey("gh", ""))
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
	cache.Set(CacheKey("gh", ""), &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Fields:     map[string]string{"access_token": "cached_tok"},
	})

	r := &stubResolver{cred: &Credential{Data: map[string]string{"access_token": "fresh_tok"}}}
	values, err := ResolveBindings(context.Background(), cache, "gh", "", bindings("access_token"), r)
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
	cache.Set(CacheKey("aws", ""), &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Fields:     map[string]string{"access_key_id": "AKIA"},
	})

	r := &stubResolver{cred: &Credential{
		Data: map[string]string{
			"access_key_id":     "AKIA-fresh",
			"secret_access_key": "secret-fresh",
		},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}

	values, err := ResolveBindings(context.Background(), cache, "aws", "", bindings("access_key_id", "secret_access_key"), r)
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
	cache.Set(CacheKey("gh", ""), &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      true,
		Fields:     map[string]string{"access_token": "stale_tok"},
	})

	r := &stubResolver{cred: &Credential{
		Data:       map[string]string{"access_token": "fresh_tok"},
		CacheUntil: time.Now().Add(10 * time.Minute),
	}}
	values, err := ResolveBindings(context.Background(), cache, "gh", "", bindings("access_token"), r)
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
	values, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", "", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 1 || values[0] != "" {
		t.Errorf("values = %v, want [\"\"] for nil credential", values)
	}
}

func TestResolveBindings_ResolverError(t *testing.T) {
	r := &stubResolver{err: errors.New("mint failed")}
	_, err := ResolveBindings(context.Background(), NewMemoryCache(), "gh", "", bindings("access_token"), r)
	if err == nil {
		t.Fatal("expected error from resolver to propagate")
	}
}

func TestResolveBindings_ZeroCacheUntilNotReused(t *testing.T) {
	cache := NewMemoryCache()
	r := &stubResolver{cred: &Credential{
		Data: map[string]string{"access_token": "tok"},
		// CacheUntil zero => ephemeral, not reused on next call.
	}}

	if _, err := ResolveBindings(context.Background(), cache, "gh", "", bindings("access_token"), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second call must re-resolve because the written entry is immediately expired.
	if _, err := ResolveBindings(context.Background(), cache, "gh", "", bindings("access_token"), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 2 {
		t.Errorf("resolver called %d times, want 2 (zero CacheUntil must not be reused)", r.calls)
	}
}

// accountResolver returns a distinct credential per account, recording calls.
type accountResolver struct {
	byAccount map[string]*Credential
	calls     int
	accounts  []string
}

func (r *accountResolver) Resolve(ctx context.Context, tool Tool, account string) (*Credential, error) {
	r.calls++
	r.accounts = append(r.accounts, account)
	cred, ok := r.byAccount[account]
	if !ok {
		return nil, errors.New("unknown account")
	}
	return cred, nil
}

func TestCacheKey_CollisionFree(t *testing.T) {
	if CacheKey("slack", "a1") == CacheKey("slack", "a2") {
		t.Error("distinct accounts of one tool must not share a cache key")
	}
	if CacheKey("slack", "") == CacheKey("slack", "a1") {
		t.Error("the default account must not share a key with a named account")
	}
	if CacheKey("slack", "a") == CacheKey("slacka", "") {
		t.Error("tool/account boundary must be unambiguous")
	}
	if got, want := CacheKey("gh", "work"), "gh\x00work"; got != want {
		t.Errorf("CacheKey = %q, want %q", got, want)
	}
}

func TestResolveBindings_DistinctAccountsDoNotShareCache(t *testing.T) {
	cache := NewMemoryCache()
	r := &accountResolver{byAccount: map[string]*Credential{
		"a1": {Data: map[string]string{"access_token": "tok-a1"}, CacheUntil: time.Now().Add(time.Hour)},
		"a2": {Data: map[string]string{"access_token": "tok-a2"}, CacheUntil: time.Now().Add(time.Hour)},
	}}

	v1, err := ResolveBindings(context.Background(), cache, "slack", "a1", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v2, err := ResolveBindings(context.Background(), cache, "slack", "a2", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v1[0] != "tok-a1" || v2[0] != "tok-a2" {
		t.Errorf("values = %q/%q, want tok-a1/tok-a2", v1[0], v2[0])
	}
	// a1's fresh cache entry must NOT satisfy a2: the resolver runs once per account.
	if r.calls != 2 {
		t.Errorf("resolver called %d times, want 2 (one per account)", r.calls)
	}

	// Repeat for a1: served from the per-account cache, no extra resolve.
	v1again, err := ResolveBindings(context.Background(), cache, "slack", "a1", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v1again[0] != "tok-a1" {
		t.Errorf("cached value = %q, want tok-a1", v1again[0])
	}
	if r.calls != 2 {
		t.Errorf("resolver called %d times after cache hit, want still 2", r.calls)
	}
}

func TestResolveBindings_ExpiredAccountEntryReResolves(t *testing.T) {
	cache := NewMemoryCache()
	// Pre-seed an EXPIRED entry for (slack, a1).
	cache.Set(CacheKey("slack", "a1"), &CacheEntry{
		FetchedAt:  time.Now().Add(-2 * time.Hour),
		CacheUntil: time.Now().Add(-time.Hour),
		Fields:     map[string]string{"access_token": "expired-tok"},
	})
	r := &accountResolver{byAccount: map[string]*Credential{
		"a1": {Data: map[string]string{"access_token": "fresh-tok"}, CacheUntil: time.Now().Add(time.Hour)},
	}}

	values, err := ResolveBindings(context.Background(), cache, "slack", "a1", bindings("access_token"), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (expired CacheUntil must re-resolve)", r.calls)
	}
	if values[0] != "fresh-tok" {
		t.Errorf("values[0] = %q, want fresh-tok", values[0])
	}
	if len(r.accounts) != 1 || r.accounts[0] != "a1" {
		t.Errorf("resolver asked for accounts %v, want [a1]", r.accounts)
	}
}
