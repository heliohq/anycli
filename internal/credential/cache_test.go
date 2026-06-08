package credential

import (
	"testing"
	"time"
)

func TestMemoryCache_SetGet_RoundTrip(t *testing.T) {
	c := NewMemoryCache()

	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      false,
		Fields: map[string]string{
			"access_token":  "tok_abc",
			"refresh_token": "ref_xyz",
		},
	}
	c.Set("github", entry)

	got, ok := c.Get("github")
	if !ok {
		t.Fatal("Get returned ok=false for a set entry")
	}
	if got == nil {
		t.Fatal("Get returned nil entry")
	}
	if got.Fields["access_token"] != "tok_abc" {
		t.Errorf("access_token = %q, want %q", got.Fields["access_token"], "tok_abc")
	}
	if got.Fields["refresh_token"] != "ref_xyz" {
		t.Errorf("refresh_token = %q, want %q", got.Fields["refresh_token"], "ref_xyz")
	}
}

func TestMemoryCache_Get_Missing(t *testing.T) {
	c := NewMemoryCache()

	got, ok := c.Get("no-tool")
	if ok {
		t.Error("Get returned ok=true for a missing entry")
	}
	if got != nil {
		t.Errorf("Get returned non-nil for a missing entry: %v", got)
	}
}

func TestMemoryCache_Set_Replaces(t *testing.T) {
	c := NewMemoryCache()
	c.Set("gh", &CacheEntry{Fields: map[string]string{"access_token": "old"}})
	c.Set("gh", &CacheEntry{Fields: map[string]string{"access_token": "new"}})

	got, ok := c.Get("gh")
	if !ok || got.Fields["access_token"] != "new" {
		t.Errorf("Set did not replace: got %v", got)
	}
}

func TestMemoryCache_MarkStale(t *testing.T) {
	c := NewMemoryCache()

	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      false,
		Fields:     map[string]string{"token": "secret"},
	}
	c.Set("tool-a", entry)

	c.MarkStale("tool-a")

	got, ok := c.Get("tool-a")
	if !ok || got == nil {
		t.Fatal("Get returned no entry after MarkStale")
	}
	if !got.Stale {
		t.Error("Stale = false after MarkStale, want true")
	}
	if got.Fields["token"] != "secret" {
		t.Errorf("Fields not preserved: token = %q, want %q", got.Fields["token"], "secret")
	}
	if got.IsValid() {
		t.Error("IsValid() = true after MarkStale, want false")
	}
}

func TestMemoryCache_MarkStale_Missing(t *testing.T) {
	c := NewMemoryCache()
	// MarkStale on a non-existent entry must be a no-op (no panic, nothing set).
	c.MarkStale("no-tool")
	if _, ok := c.Get("no-tool"); ok {
		t.Error("MarkStale on a missing entry must not create one")
	}
}

func TestIsValid_FreshCache(t *testing.T) {
	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      false,
		Fields:     map[string]string{"key": "val"},
	}
	if !entry.IsValid() {
		t.Error("IsValid() = false for fresh cache, want true")
	}
}

func TestIsValid_ExpiredCache(t *testing.T) {
	entry := &CacheEntry{
		FetchedAt:  time.Now().Add(-20 * time.Minute),
		CacheUntil: time.Now().Add(-5 * time.Minute),
		Stale:      false,
		Fields:     map[string]string{"key": "val"},
	}
	if entry.IsValid() {
		t.Error("IsValid() = true for expired cache, want false")
	}
}

func TestIsValid_StaleCache(t *testing.T) {
	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      true,
		Fields:     map[string]string{"key": "val"},
	}
	if entry.IsValid() {
		t.Error("IsValid() = true for stale cache, want false")
	}
}
