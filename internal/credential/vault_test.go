package credential

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFromVault_EmptyResponse_ReturnsNil(t *testing.T) {
	// Regression: empty vault response should return nil, nil (not an error).
	// Tools may work without auth; an empty vault response is not fatal.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"credentials": []interface{}{},
		})
	}))
	defer server.Close()

	cfg := &VaultConfig{
		URL:         server.URL,
		Token:       "test-token",
		WorkspaceID: "ws-1",
	}

	cred, err := FetchFromVault(cfg, "nonexistent-tool")
	if err != nil {
		t.Fatalf("FetchFromVault should return nil error for empty response, got: %v", err)
	}
	if cred != nil {
		t.Errorf("FetchFromVault should return nil credential for empty response, got: %+v", cred)
	}
}

func TestFetchFromVault_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", auth, "Bearer test-token")
		}

		// Verify query params
		if r.URL.Query().Get("workspace_id") != "ws-1" {
			t.Errorf("workspace_id = %q, want %q", r.URL.Query().Get("workspace_id"), "ws-1")
		}
		if r.URL.Query().Get("tool") != "github" {
			t.Errorf("tool = %q, want %q", r.URL.Query().Get("tool"), "github")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"credentials": []interface{}{
				map[string]interface{}{
					"id":     "cred-1",
					"tool":   "github",
					"source": "user",
					"type":   "token",
					"data": map[string]interface{}{
						"access_token": "ghp_test123",
					},
					"status":      "active",
					"cache_until": "2026-12-31T00:00:00Z",
				},
			},
		})
	}))
	defer server.Close()

	cfg := &VaultConfig{
		URL:         server.URL,
		Token:       "test-token",
		WorkspaceID: "ws-1",
	}

	cred, err := FetchFromVault(cfg, "github")
	if err != nil {
		t.Fatalf("FetchFromVault returned error: %v", err)
	}
	if cred == nil {
		t.Fatal("FetchFromVault returned nil credential")
	}
	if cred.Tool != "github" {
		t.Errorf("Tool = %q, want %q", cred.Tool, "github")
	}
	if cred.Data["access_token"] != "ghp_test123" {
		t.Errorf("Data.access_token = %v, want %q", cred.Data["access_token"], "ghp_test123")
	}
	if cred.CacheUntil == nil || *cred.CacheUntil != "2026-12-31T00:00:00Z" {
		t.Errorf("CacheUntil = %v, want %q", cred.CacheUntil, "2026-12-31T00:00:00Z")
	}
}

func TestFetchFromVault_4xx_ReturnsNonTransientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid token"}`))
	}))
	defer server.Close()

	cfg := &VaultConfig{
		URL:         server.URL,
		Token:       "bad-token",
		WorkspaceID: "ws-1",
	}

	_, err := FetchFromVault(cfg, "github")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	// 4xx errors should NOT be VaultFetchError (no stale cache fallback)
	if _, ok := err.(*VaultFetchError); ok {
		t.Error("4xx errors should NOT be VaultFetchError (no cache fallback)")
	}
}

func TestFetchFromVault_5xx_ReturnsVaultFetchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer server.Close()

	cfg := &VaultConfig{
		URL:         server.URL,
		Token:       "token",
		WorkspaceID: "ws-1",
	}

	_, err := FetchFromVault(cfg, "github")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if _, ok := err.(*VaultFetchError); !ok {
		t.Errorf("5xx errors should be VaultFetchError for stale cache fallback, got %T", err)
	}
}
