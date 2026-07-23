package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveFromGateway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections/token" {
			t.Errorf("path = %q, want /connections/token", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-e2e-test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("provider"); got != "google_drive" {
			t.Errorf("provider = %q, want google_drive (mapped from drive)", got)
		}
		if got := r.URL.Query().Get("account"); got != "secondary" {
			t.Errorf("account = %q, want secondary", got)
		}
		exp := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"access_token": "tok-123",
			"expires_at":   exp,
			"credential":   map[string]string{"access_token": "tok-123", "subject": "a@b.c"},
		}})
	}))
	defer srv.Close()

	t.Setenv("HELIO_E2E_API_KEY", "sk-e2e-test")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, err := NewResolver()
	if err != nil {
		t.Fatal(err)
	}
	cred, err := r.Resolve(context.Background(), "drive", "secondary")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Data["access_token"] != "tok-123" || cred.Data["subject"] != "a@b.c" {
		t.Errorf("Data = %v", cred.Data)
	}
	if !cred.CacheUntil.After(time.Now()) || !cred.CacheUntil.Before(time.Now().Add(30*time.Minute)) {
		t.Errorf("CacheUntil = %v, want inside (now, expires_at) with safety margin", cred.CacheUntil)
	}
}

func TestResolveAccessTokenOnlyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"access_token": "tok-9"}})
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	cred, err := r.Resolve(context.Background(), "attio", "")
	if err != nil {
		t.Fatal(err)
	}
	// Empty credential map falls back to {"access_token": ...}, and a
	// response without expires_at gets the default 50-minute horizon.
	if cred.Data["access_token"] != "tok-9" {
		t.Errorf("Data = %v", cred.Data)
	}
}

func TestResolveNotConnectedIs404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"not_found"}}`, http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	_, err := r.Resolve(context.Background(), "attio", "")
	if !IsNotConnected(err) {
		t.Fatalf("err = %v, want NotConnectedError", err)
	}
}

func TestResolveOtherHTTPErrorsAreNotSkips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"unauthorized"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	_, err := r.Resolve(context.Background(), "attio", "")
	if err == nil || IsNotConnected(err) {
		t.Fatalf("err = %v, want a hard (non-skip) error", err)
	}
}

func TestEnvOverrideBeatsGateway(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	t.Setenv("ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN", "local-tok")
	t.Setenv("ANYCLI_E2E_CRED_SECONDARY_ACCESS_TOKEN", "local-tok-2")
	r, _ := NewResolver()

	cred, err := r.Resolve(context.Background(), "attio", "")
	if err != nil || cred.Data["access_token"] != "local-tok" {
		t.Fatalf("default account: cred=%v err=%v", cred, err)
	}
	cred, err = r.Resolve(context.Background(), "attio", "secondary")
	if err != nil || cred.Data["access_token"] != "local-tok-2" {
		t.Fatalf("secondary account: cred=%v err=%v", cred, err)
	}
	if called {
		t.Fatal("gateway must not be called when the env override is set")
	}
}

func TestNewResolverRequiresConfig(t *testing.T) {
	t.Setenv("HELIO_E2E_API_KEY", "")
	t.Setenv("HELIO_E2E_API_BASE", "")
	if _, err := NewResolver(); err == nil {
		t.Fatal("want error when HELIO_E2E_API_KEY / HELIO_E2E_API_BASE are unset")
	}
}
