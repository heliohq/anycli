package serpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- search ---

func TestSearch_Happy_InjectsAPIKeyAsQueryParam(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"search_metadata":{"id":"abc123","status":"Success"},"organic_results":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "search", "-q", "coffee")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/search" {
		t.Errorf("request = %s %s, want GET /search", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("api_key") != "key-123" {
		t.Errorf("api_key = %q, want key-123 as query param", q.Get("api_key"))
	}
	if q.Get("q") != "coffee" {
		t.Errorf("q = %q, want coffee", q.Get("q"))
	}
	if q.Get("engine") != "google" {
		t.Errorf("engine = %q, want the google default", q.Get("engine"))
	}
	if !strings.Contains(stdout, `"organic_results"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestSearch_FlagToParamMapping(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"search", "-q", "coffee", "--engine", "google_news",
		"--location", "Austin, Texas, United States",
		"--gl", "us", "--hl", "en", "--google-domain", "google.com",
		"--device", "mobile", "--num", "5", "--start", "10", "--no-cache")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	want := map[string]string{
		"engine":        "google_news",
		"q":             "coffee",
		"location":      "Austin, Texas, United States",
		"gl":            "us",
		"hl":            "en",
		"google_domain": "google.com",
		"device":        "mobile",
		"num":           "5",
		"start":         "10",
		"no_cache":      "true",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
}

func TestSearch_OmitsUnsetOptionalParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "-q", "coffee")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	for _, k := range []string{"location", "gl", "hl", "google_domain", "device", "num", "start", "no_cache"} {
		if q.Has(k) {
			t.Errorf("unset flag leaked query param %s=%q", k, q.Get(k))
		}
	}
}

func TestSearch_ParamEscapeHatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "-q", "pizza",
		"--param", "tbm=nws", "--param", "safe=active", "--param", "filter=")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("tbm") != "nws" {
		t.Errorf("tbm = %q, want nws", q.Get("tbm"))
	}
	if q.Get("safe") != "active" {
		t.Errorf("safe = %q, want active", q.Get("safe"))
	}
	if !q.Has("filter") || q.Get("filter") != "" {
		t.Errorf("filter = %q (has=%t), want present and empty", q.Get("filter"), q.Has("filter"))
	}
}

func TestSearch_ParamOverridesFlag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "-q", "pizza", "--gl", "us", "--param", "gl=fr")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("gl") != "fr" {
		t.Errorf("gl = %q, want the --param override fr", q.Get("gl"))
	}
}

func TestSearch_ParamMissingEquals_IsUsageError(t *testing.T) {
	result, _, stderr := runResult(t, nil, "search", "-q", "x", "--param", "novalue")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if !strings.Contains(stderr, "key=value") {
		t.Errorf("stderr = %q, want the key=value usage hint", stderr)
	}
}

func TestSearch_ParamCannotOverrideAPIKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "-q", "x", "--param", "api_key=stolen")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("api_key") != "key-123" {
		t.Errorf("api_key = %q, want the resolved credential key-123", q.Get("api_key"))
	}
}

// --- archive ---

func TestArchiveGet_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"search_metadata":{"id":"abc123"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "archive", "get", "abc123")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/searches/abc123.json" {
		t.Errorf("request = %s %s, want GET /searches/abc123.json", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("api_key") != "key-123" {
		t.Errorf("api_key = %q, want key-123 as query param", q.Get("api_key"))
	}
	if !strings.Contains(stdout, `"abc123"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestArchiveGet_MissingID_IsUsageError(t *testing.T) {
	result, _, _ := runResult(t, nil, "archive", "get")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
}

// --- locations ---

func TestLocations_SendsNoAPIKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"name":"Austin","canonical_name":"Austin,TX,Texas,United States"}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "locations", "--q", "austin", "--limit", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/locations.json" {
		t.Errorf("request = %s %s, want GET /locations.json", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Has("api_key") {
		t.Errorf("api_key leaked to the free Locations API: %q", q.Get("api_key"))
	}
	if q.Get("q") != "austin" || q.Get("limit") != "3" {
		t.Errorf("query = %q, want q=austin&limit=3", got.Query)
	}
	if !strings.Contains(stdout, "canonical_name") {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestLocations_WorksWithoutCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	result, stdout, _ := runWithEnv(t, srv, map[string]string{}, "locations", "--q", "berlin")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (locations needs no credential)", result.ExitCode)
	}
	if !strings.Contains(stdout, "[]") {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

// --- account ---

func TestAccount_RedactsAPIKeyField(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"account_id":"acct1","api_key":"key-123","account_email":"a@b.c","total_searches_left":95}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/account.json" {
		t.Errorf("path = %s, want /account.json", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("api_key") != "key-123" {
		t.Errorf("api_key = %q, want key-123 as query param", q.Get("api_key"))
	}
	if strings.Contains(stdout, "key-123") {
		t.Errorf("stdout leaks the private API key: %q", stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	if _, present := out["api_key"]; present {
		t.Error("api_key field must be removed from account output")
	}
	if out["account_id"] != "acct1" || out["account_email"] != "a@b.c" {
		t.Errorf("account fields dropped: %v", out)
	}
	if out["total_searches_left"] != float64(95) {
		t.Errorf("total_searches_left = %v, want 95", out["total_searches_left"])
	}
}

// --- credentials & errors ---

func TestMissingAPIKey_AuthedCommandFails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, stderr := runWithEnv(t, srv, map[string]string{}, "search", "-q", "x")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "SERPAPI_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-key message", stderr)
	}
	if got.Path != "" {
		t.Errorf("request reached the server (%s) despite the missing key", got.Path)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "bad request", status: http.StatusBadRequest, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"error":"Invalid API key. Your API key should be here: https://serpapi.com/manage-api-key"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "account")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "Invalid API key") {
				t.Errorf("stderr = %q, want the provider error string", stderr)
			}
		})
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, "{\"error\":\"Missing query `q` parameter.\"}", &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", err, stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind api status 400", envelope.Error)
	}
	if !strings.Contains(envelope.Error.Message, "Missing query") {
		t.Errorf("message = %q, want the provider error string", envelope.Error.Message)
	}
}

func TestUsageError_JSONEnvelope(t *testing.T) {
	result, _, stderr := runResult(t, nil, "search", "--json", "--param", "broken")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", err, stderr)
	}
	if envelope.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", envelope.Error.Kind)
	}
}

func TestUnknownSubcommand_IsUsageError(t *testing.T) {
	result, _, _ := runResult(t, nil, "does-not-exist")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
}

func TestNonJSONErrorBody_FallsBackToRawBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusServiceUnavailable, `upstream exploded`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "upstream exploded") {
		t.Errorf("stderr = %q, want the raw body fallback", stderr)
	}
}
