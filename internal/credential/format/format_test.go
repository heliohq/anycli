package format

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- JSON tests ---

func TestJSON_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	fields := map[string]string{
		"github.com.oauth_token": "ghp_abc123",
		"github.com.user":       "testuser",
	}

	if err := PatchFile(path, "json", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	gh, ok := m["github"].(map[string]interface{})
	if !ok {
		t.Fatal("expected github key")
	}
	com, ok := gh["com"].(map[string]interface{})
	if !ok {
		t.Fatal("expected github.com key")
	}
	if com["oauth_token"] != "ghp_abc123" {
		t.Errorf("oauth_token = %v, want ghp_abc123", com["oauth_token"])
	}
	if com["user"] != "testuser" {
		t.Errorf("user = %v, want testuser", com["user"])
	}
}

func TestJSON_PatchExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := `{
  "github": {
    "com": {
      "oauth_token": "old_token",
      "user": "olduser"
    }
  },
  "other": "value"
}`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"github.com.oauth_token": "new_token",
	}

	if err := PatchFile(path, "json", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	gh := m["github"].(map[string]interface{})
	com := gh["com"].(map[string]interface{})
	if com["oauth_token"] != "new_token" {
		t.Errorf("oauth_token = %v, want new_token", com["oauth_token"])
	}
	if com["user"] != "olduser" {
		t.Errorf("user = %v, want olduser (should be preserved)", com["user"])
	}
	if m["other"] != "value" {
		t.Errorf("other = %v, want value (should be preserved)", m["other"])
	}
}

func TestJSON_CleanupFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := `{
  "github": {
    "com": {
      "oauth_token": "token",
      "user": "testuser"
    }
  }
}`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"github.com.oauth_token": "token",
	}

	if err := CleanupFields(path, "json", fields); err != nil {
		t.Fatalf("CleanupFields: %v", err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	gh := m["github"].(map[string]interface{})
	com := gh["com"].(map[string]interface{})
	if _, ok := com["oauth_token"]; ok {
		t.Error("oauth_token should have been removed")
	}
	if com["user"] != "testuser" {
		t.Errorf("user = %v, want testuser (should be preserved)", com["user"])
	}
}

func TestJSON_CleanupPrunesEmptyParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := `{
  "github": {
    "com": {
      "oauth_token": "token"
    }
  }
}`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"github.com.oauth_token": "token",
	}

	if err := CleanupFields(path, "json", fields); err != nil {
		t.Fatalf("CleanupFields: %v", err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if _, ok := m["github"]; ok {
		t.Error("github should have been pruned (empty after removal)")
	}
}

// --- YAML tests ---

func TestYAML_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	fields := map[string]string{
		"github.com.oauth_token": "ghp_abc123",
		"github.com.user":       "testuser",
	}

	if err := PatchFile(path, "yaml", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "github:") {
		t.Error("should contain 'github:' section")
	}
	if !strings.Contains(content, "oauth_token: ghp_abc123") {
		t.Error("should contain 'oauth_token: ghp_abc123'")
	}
	if !strings.Contains(content, "user: testuser") {
		t.Error("should contain 'user: testuser'")
	}
}

func TestYAML_PatchExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := `github:
  com:
    oauth_token: old_token
    user: olduser
other: value
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"github.com.oauth_token": "new_token",
	}

	if err := PatchFile(path, "yaml", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "oauth_token: new_token") {
		t.Errorf("should contain patched value, got:\n%s", content)
	}
	if !strings.Contains(content, "user: olduser") {
		t.Errorf("should preserve existing values, got:\n%s", content)
	}
	if !strings.Contains(content, "other: value") {
		t.Errorf("should preserve unrelated keys, got:\n%s", content)
	}
}

func TestYAML_RemoveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := `github:
  com:
    oauth_token: secret
    user: testuser
other: value
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"github.com.oauth_token": "secret",
	}

	if err := CleanupFields(path, "yaml", fields); err != nil {
		t.Fatalf("CleanupFields: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if strings.Contains(content, "oauth_token") {
		t.Errorf("oauth_token should have been removed, got:\n%s", content)
	}
	if !strings.Contains(content, "user: testuser") {
		t.Errorf("user should be preserved, got:\n%s", content)
	}
}

func TestYAML_QuotesSpecialValues(t *testing.T) {
	h := yamlHandler{}
	fields := map[string]string{
		"key1": "true",
		"key2": "value: with colon",
		"key3": "",
	}
	data, err := h.Create(fields)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `key1: "true"`) {
		t.Errorf("should quote 'true', got:\n%s", content)
	}
	if !strings.Contains(content, `key2: "value: with colon"`) {
		t.Errorf("should quote value with colon, got:\n%s", content)
	}
	if !strings.Contains(content, `key3: ""`) {
		t.Errorf("should quote empty string, got:\n%s", content)
	}
}

// --- TOML tests ---

func TestTOML_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	fields := map[string]string{
		"auth.token":    "abc123",
		"auth.username": "testuser",
		"debug":         "true",
	}

	if err := PatchFile(path, "toml", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "[auth]") {
		t.Error("should contain [auth] section")
	}
	if !strings.Contains(content, `token = "abc123"`) {
		t.Errorf("should contain token, got:\n%s", content)
	}
	if !strings.Contains(content, `debug = "true"`) {
		t.Errorf("should contain debug, got:\n%s", content)
	}
}

func TestTOML_PatchExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	initial := `[auth]
token = "old_token"
username = "olduser"
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"auth.token": "new_token",
	}

	if err := PatchFile(path, "toml", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, `token = "new_token"`) {
		t.Errorf("should contain patched token, got:\n%s", content)
	}
	if !strings.Contains(content, `username = "olduser"`) {
		t.Errorf("should preserve username, got:\n%s", content)
	}
}

func TestTOML_RemoveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	initial := `[auth]
token = "secret"
username = "testuser"
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"auth.token": "secret",
	}

	if err := CleanupFields(path, "toml", fields); err != nil {
		t.Fatalf("CleanupFields: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if strings.Contains(content, "token") {
		t.Errorf("token should have been removed, got:\n%s", content)
	}
	if !strings.Contains(content, `username = "testuser"`) {
		t.Errorf("username should be preserved, got:\n%s", content)
	}
}

// --- INI tests ---

func TestINI_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	fields := map[string]string{
		"default.region":   "us-east-1",
		"default.output":   "json",
		"profile.dev.role": "arn:aws:iam::role/dev",
	}

	if err := PatchFile(path, "ini", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "[default]") {
		t.Error("should contain [default] section")
	}
	if !strings.Contains(content, "region = us-east-1") {
		t.Errorf("should contain region, got:\n%s", content)
	}
	if !strings.Contains(content, "[profile.dev]") {
		t.Errorf("should contain [profile.dev] section, got:\n%s", content)
	}
	if !strings.Contains(content, "role = arn:aws:iam::role/dev") {
		t.Errorf("should contain role, got:\n%s", content)
	}
}

func TestINI_PatchExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	initial := `[default]
region = us-west-2
output = json
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"default.region": "us-east-1",
	}

	if err := PatchFile(path, "ini", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "region = us-east-1") {
		t.Errorf("should contain patched region, got:\n%s", content)
	}
	if !strings.Contains(content, "output = json") {
		t.Errorf("should preserve output, got:\n%s", content)
	}
}

func TestINI_RemoveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	initial := `[default]
region = us-east-1
output = json
`
	os.WriteFile(path, []byte(initial), 0600)

	fields := map[string]string{
		"default.region": "us-east-1",
	}

	if err := CleanupFields(path, "ini", fields); err != nil {
		t.Fatalf("CleanupFields: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if strings.Contains(content, "region") {
		t.Errorf("region should have been removed, got:\n%s", content)
	}
	if !strings.Contains(content, "output = json") {
		t.Errorf("output should be preserved, got:\n%s", content)
	}
}

// --- Edge case tests ---

func TestPatchFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "config.json")

	fields := map[string]string{"key": "value"}
	if err := PatchFile(path, "json", fields, 0600); err != nil {
		t.Fatalf("PatchFile should create parent dirs: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

func TestPatchFile_UnsupportedFormat(t *testing.T) {
	if err := PatchFile("/tmp/test", "xml", nil, 0600); err == nil {
		t.Error("should return error for unsupported format")
	}
}

func TestCleanupFields_NonexistentFile(t *testing.T) {
	if err := CleanupFields("/nonexistent/path", "json", nil); err != nil {
		t.Errorf("should silently succeed for nonexistent file: %v", err)
	}
}

func TestPatchFile_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")

	fields := map[string]string{"token": "secret"}
	if err := PatchFile(path, "json", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestINI_TopLevelKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")

	fields := map[string]string{
		"globalkey": "globalval",
	}

	if err := PatchFile(path, "ini", fields, 0600); err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "globalkey = globalval") {
		t.Errorf("should contain top-level key, got:\n%s", content)
	}
	// Should not have any section header for top-level keys.
	if strings.Contains(content, "[") {
		t.Errorf("should not contain section header for top-level key, got:\n%s", content)
	}
}
