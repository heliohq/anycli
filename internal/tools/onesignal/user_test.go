package onesignal

import (
	"net/http"
	"testing"
)

func TestUserUpsert_IdentityAndTags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"identity":{"external_id":"u-1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "upsert",
		"--alias-label", "external_id", "--alias-id", "u-1",
		"--tags", `{"plan":"pro"}`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/apps/"+testAppID+"/users" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	identity, ok := body["identity"].(map[string]any)
	if !ok || identity["external_id"] != "u-1" {
		t.Errorf("identity = %v", body["identity"])
	}
	props, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %v", body["properties"])
	}
	tags, ok := props["tags"].(map[string]any)
	if !ok || tags["plan"] != "pro" {
		t.Errorf("properties.tags = %v", props["tags"])
	}
}

func TestUserUpsert_MissingAlias_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "upsert", "--alias-label", "external_id")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected")
	}
}

func TestUserGet_AliasPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"identity":{"external_id":"u-1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get", "--alias-label", "external_id", "--alias-id", "u-1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/apps/"+testAppID+"/users/by/external_id/u-1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}
