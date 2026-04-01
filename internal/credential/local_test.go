package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ANYCLI_HOME", dir)
	return dir
}

func TestLoadLocal_ValidFile(t *testing.T) {
	home := setupHome(t)

	credDir := filepath.Join(home, "credentials")
	if err := os.MkdirAll(credDir, 0755); err != nil {
		t.Fatalf("failed to create credentials dir: %v", err)
	}

	creds := map[string]string{
		"GH_TOKEN":  "ghp_abc123",
		"API_KEY":   "key_xyz",
	}
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("failed to marshal creds: %v", err)
	}
	if err := os.WriteFile(filepath.Join(credDir, "gh.json"), data, 0600); err != nil {
		t.Fatalf("failed to write cred file: %v", err)
	}

	got, err := LoadLocal("gh")
	if err != nil {
		t.Fatalf("LoadLocal returned error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadLocal returned nil map")
	}
	if got["GH_TOKEN"] != "ghp_abc123" {
		t.Errorf("GH_TOKEN = %q, want %q", got["GH_TOKEN"], "ghp_abc123")
	}
	if got["API_KEY"] != "key_xyz" {
		t.Errorf("API_KEY = %q, want %q", got["API_KEY"], "key_xyz")
	}
}

func TestLoadLocal_MissingFile(t *testing.T) {
	setupHome(t)

	got, err := LoadLocal("nonexistent")
	if err != nil {
		t.Fatalf("LoadLocal returned error for missing file: %v", err)
	}
	if got != nil {
		t.Errorf("LoadLocal returned non-nil map for missing file: %v", got)
	}
}

func TestLoadLocal_InvalidJSON(t *testing.T) {
	home := setupHome(t)

	credDir := filepath.Join(home, "credentials")
	if err := os.MkdirAll(credDir, 0755); err != nil {
		t.Fatalf("failed to create credentials dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(credDir, "broken.json"), []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("failed to write broken file: %v", err)
	}

	got, err := LoadLocal("broken")
	if err == nil {
		t.Fatalf("LoadLocal should return error for invalid JSON, got map: %v", got)
	}
}
