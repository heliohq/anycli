package onesignal

import (
	"net/http"
	"testing"
)

func TestSegmentCreate_AppScopedPathAndBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"seg-1","name":"VIPs"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "segment", "create",
		"--name", "VIPs",
		"--filters", `[{"field":"session_count","relation":">","value":"10"}]`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/apps/"+testAppID+"/segments" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.Auth != "Key "+testKey {
		t.Errorf("Authorization = %q", got.Auth)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "VIPs" {
		t.Errorf("name = %v", body["name"])
	}
	if _, ok := body["filters"].([]any); !ok {
		t.Errorf("filters = %v", body["filters"])
	}
}

func TestSegmentCreate_BadFiltersJSON_UsageExit2(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	code, _, _ := run(t, srv, "segment", "create", "--name", "X", "--filters", "not-json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected on bad JSON")
	}
}

func TestSegmentList_AppScopedPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"segments":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "segment", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/apps/"+testAppID+"/segments" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestSegmentDelete_AppScopedPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":"true"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "segment", "delete", "--id", "seg-9")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/apps/"+testAppID+"/segments/seg-9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}
