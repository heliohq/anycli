package credential

import (
	"testing"
	"time"
)

func TestWriteCache_ReadCache_RoundTrip(t *testing.T) {
	setupHome(t)

	entry := &CacheEntry{
		FetchedAt:  time.Now().Truncate(time.Second),
		CacheUntil: time.Now().Add(10 * time.Minute).Truncate(time.Second),
		Stale:      false,
		Fields: map[string]string{
			"access_token": "tok_abc",
			"refresh_token": "ref_xyz",
		},
	}

	tokenHash := TokenFingerprint("test-token")

	if err := WriteCache("ws-123", tokenHash, "github", entry); err != nil {
		t.Fatalf("WriteCache failed: %v", err)
	}

	got, err := ReadCache("ws-123", tokenHash, "github")
	if err != nil {
		t.Fatalf("ReadCache failed: %v", err)
	}
	if got == nil {
		t.Fatal("ReadCache returned nil")
	}

	if got.Stale != false {
		t.Errorf("Stale = %v, want false", got.Stale)
	}
	if got.Fields["access_token"] != "tok_abc" {
		t.Errorf("access_token = %q, want %q", got.Fields["access_token"], "tok_abc")
	}
	if got.Fields["refresh_token"] != "ref_xyz" {
		t.Errorf("refresh_token = %q, want %q", got.Fields["refresh_token"], "ref_xyz")
	}
	// Time comparison: truncate to second to avoid JSON marshal/unmarshal precision issues
	if !got.FetchedAt.Truncate(time.Second).Equal(entry.FetchedAt) {
		t.Errorf("FetchedAt = %v, want %v", got.FetchedAt, entry.FetchedAt)
	}
	if !got.CacheUntil.Truncate(time.Second).Equal(entry.CacheUntil) {
		t.Errorf("CacheUntil = %v, want %v", got.CacheUntil, entry.CacheUntil)
	}
}

func TestReadCache_MissingFile(t *testing.T) {
	setupHome(t)

	got, err := ReadCache("ws-nonexistent", "abcd1234", "no-tool")
	if err != nil {
		t.Fatalf("ReadCache returned error for missing file: %v", err)
	}
	if got != nil {
		t.Errorf("ReadCache returned non-nil for missing file: %v", got)
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

func TestMarkStale(t *testing.T) {
	setupHome(t)

	tokenHash := TokenFingerprint("test-token")

	entry := &CacheEntry{
		FetchedAt:  time.Now(),
		CacheUntil: time.Now().Add(10 * time.Minute),
		Stale:      false,
		Fields:     map[string]string{"token": "secret"},
	}
	if err := WriteCache("ws-mark", tokenHash, "tool-a", entry); err != nil {
		t.Fatalf("WriteCache failed: %v", err)
	}

	if err := MarkStale("ws-mark", tokenHash, "tool-a"); err != nil {
		t.Fatalf("MarkStale failed: %v", err)
	}

	got, err := ReadCache("ws-mark", tokenHash, "tool-a")
	if err != nil {
		t.Fatalf("ReadCache after MarkStale failed: %v", err)
	}
	if got == nil {
		t.Fatal("ReadCache returned nil after MarkStale")
	}
	if !got.Stale {
		t.Error("Stale = false after MarkStale, want true")
	}
	if got.Fields["token"] != "secret" {
		t.Errorf("Fields preserved: token = %q, want %q", got.Fields["token"], "secret")
	}

	// After marking stale, IsValid should return false
	if got.IsValid() {
		t.Error("IsValid() = true after MarkStale, want false")
	}
}

func TestMarkStale_NonexistentCache(t *testing.T) {
	setupHome(t)

	// MarkStale on a non-existent cache should not error
	if err := MarkStale("ws-nope", "abcd1234", "no-tool"); err != nil {
		t.Fatalf("MarkStale on nonexistent cache should not error, got: %v", err)
	}
}
