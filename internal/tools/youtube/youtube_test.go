package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"channels", "get", "--mine"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "YOUTUBE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingTokenJSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"channels", "get", "--mine", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(errBuf.Bytes(), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "YOUTUBE_ACCESS_TOKEN") {
		t.Errorf("envelope = %+v, want usage kind + token message", env.Error)
	}
}

func TestBearerInjection(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/channels": {http.StatusOK, `{"items":[{"id":"UC1","snippet":{"title":"Helio"},"statistics":{"subscriberCount":"10","viewCount":"99","videoCount":"3"}}]}`},
	})
	f.runOK(t, "channels", "get", "--mine")
	got := f.last(t, "GET", "/youtube/v3/channels")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
}

func TestArgvParsing_Failures_Exit2(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"videos", "explode"}, "explode"},
		{"channels get without selector", []string{"channels", "get"}, "one of --mine"},
		{"search without query", []string{"search"}, "--query is required"},
		{"search bad type", []string{"search", "--query", "x", "--type", "movie"}, "--type must be"},
		{"videos get without id", []string{"videos", "get"}, "--id is required"},
		{"videos update nothing", []string{"videos", "update", "--id", "v1"}, "nothing to update"},
		{"videos rate bad rating", []string{"videos", "rate", "--id", "v1", "--rating", "love"}, "--rating must be"},
		{"playlists create no title", []string{"playlists", "create"}, "--title is required"},
		{"playlist-items add missing", []string{"playlist-items", "add", "--playlist", "p1"}, "--playlist and --video are required"},
		{"comments moderate bad status", []string{"comments", "moderate", "--id", "c1", "--status", "burn"}, "--status must be"},
		{"comments moderate ban without reject", []string{"comments", "moderate", "--id", "c1", "--status", "published", "--ban-author"}, "--ban-author is only valid with --status rejected"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 2 {
				t.Fatalf("exit code = %d, want 2 (usage)", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestAPIError_Exit1_WithScopeHint(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/channels": {http.StatusForbidden, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"insufficient authentication scopes","errors":[{"reason":"insufficientPermissions"}]}}`},
	})
	result, _, stderr := f.run(t, "channels", "get", "--mine")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (api)", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}
}

func TestAPIError_JSONEnvelopeCarriesStatus(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/videos": {http.StatusNotFound, `{"error":{"code":404,"status":"NOT_FOUND","message":"video not found"}}`},
	})
	result, _, stderr := f.run(t, "videos", "get", "--id", "nope", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Errorf("envelope = %+v, want api kind + status 404", env.Error)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "UNAUTHENTICATED", true},
		{"explicit unauthenticated status", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"rate limited", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", false},
		{"server failure", http.StatusInternalServerError, "INTERNAL", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /youtube/v3/channels": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "channels", "get", "--mine")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestQuotaExceeded_SurfacedVerbatimNoRetry(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/search": {http.StatusForbidden, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"The request cannot be completed because you have exceeded your quota.","errors":[{"reason":"quotaExceeded"}]}}`},
	})
	result, _, stderr := f.run(t, "search", "--query", "cats")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "exceeded your quota") || !strings.Contains(stderr, "quotaExceeded") {
		t.Errorf("stderr = %q, want the verbatim quota message + reason", stderr)
	}
	if f.count("GET", "/youtube/v3/search") != 1 {
		t.Errorf("search calls = %d, want exactly 1 (no client-side retry)", f.count("GET", "/youtube/v3/search"))
	}
	if result.CredentialRejected {
		t.Error("quotaExceeded must not reject the credential")
	}
}
