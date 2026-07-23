package dataforseo

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// --- (b) auth header + (a) one-task array body + (c) envelope unwrap ---

func TestSERPGoogle_RequestShapeAuthAndUnwrap(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0.0025", `[{"keyword":"seo tools","items_count":10}]`), &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv,
		"serp", "google", "--keyword", "seo tools", "--location", "United States", "--language", "en", "--depth", "20", "--device", "mobile")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	// (b) Authorization: Basic base64(login:password)
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	if got.Auth != wantAuth {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantAuth)
	}
	// endpoint + method
	if got.Method != http.MethodPost || got.Path != "/serp/google/organic/live/advanced" {
		t.Errorf("request = %s %s, want POST /serp/google/organic/live/advanced", got.Method, got.Path)
	}
	// (a) body is a JSON array with exactly one task object
	arr := decodeBody(t, got.Body)
	if len(arr) != 1 {
		t.Fatalf("task array length = %d, want 1", len(arr))
	}
	task := arr[0]
	if task["keyword"] != "seo tools" {
		t.Errorf("keyword = %v, want seo tools", task["keyword"])
	}
	if task["location_name"] != "United States" {
		t.Errorf("location_name = %v, want United States", task["location_name"])
	}
	if task["language_code"] != "en" {
		t.Errorf("language_code = %v, want en", task["language_code"])
	}
	if task["depth"].(float64) != 20 {
		t.Errorf("depth = %v, want 20", task["depth"])
	}
	if task["device"] != "mobile" {
		t.Errorf("device = %v, want mobile", task["device"])
	}
	// (c) envelope unwrap to {cost, result}
	out := decodeOutput(t, stdout)
	if out["cost"].(float64) != 0.0025 {
		t.Errorf("cost = %v, want 0.0025", out["cost"])
	}
	result, ok := out["result"].([]any)
	if !ok || len(result) != 1 {
		t.Fatalf("result = %v, want a one-element array", out["result"])
	}
	if _, hasVersion := out["version"]; hasVersion {
		t.Error("output leaked the DataForSEO envelope (version present)")
	}
}

// --- numeric location becomes location_code ---

func TestLocationCode_NumericGoesToLocationCode(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0.01", `[]`), &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "keywords", "volume", "--keywords", "a,b", "--location", "2840"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	task := decodeBody(t, got.Body)[0]
	if _, ok := task["location_name"]; ok {
		t.Error("numeric --location must not set location_name")
	}
	if task["location_code"].(float64) != 2840 {
		t.Errorf("location_code = %v, want 2840", task["location_code"])
	}
}

// --- keywords list splits on comma into an array field ---

func TestKeywordsVolume_KeywordsArray(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0.01", `[]`), &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "keywords", "volume", "--keywords", "seo, ppc ,link building"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	task := decodeBody(t, got.Body)[0]
	kw, ok := task["keywords"].([]any)
	if !ok {
		t.Fatalf("keywords = %v, want an array", task["keywords"])
	}
	want := []string{"seo", "ppc", "link building"}
	if len(kw) != len(want) {
		t.Fatalf("keywords = %v, want %v", kw, want)
	}
	for i, w := range want {
		if kw[i] != w {
			t.Errorf("keywords[%d] = %v, want %q (trimmed)", i, kw[i], w)
		}
	}
}

// --- endpoint routing table for the remaining commands ---

func TestEndpointRouting(t *testing.T) {
	cases := []struct {
		name string
		args []string
		path string
	}{
		{"keyword ideas", []string{"keywords", "ideas", "--keywords", "seo"}, "/dataforseo_labs/google/keyword_ideas/live"},
		{"keyword suggestions", []string{"keywords", "suggestions", "--keyword", "seo"}, "/dataforseo_labs/google/keyword_suggestions/live"},
		{"keyword difficulty", []string{"keywords", "difficulty", "--keywords", "seo"}, "/dataforseo_labs/google/bulk_keyword_difficulty/live"},
		{"search intent", []string{"keywords", "intent", "--keywords", "seo"}, "/dataforseo_labs/google/search_intent/live"},
		{"domain overview", []string{"domain", "overview", "--target", "example.com"}, "/dataforseo_labs/google/domain_rank_overview/live"},
		{"domain ranked-keywords", []string{"domain", "ranked-keywords", "--target", "example.com"}, "/dataforseo_labs/google/ranked_keywords/live"},
		{"domain competitors", []string{"domain", "competitors", "--target", "example.com"}, "/dataforseo_labs/google/competitors_domain/live"},
		{"backlinks summary", []string{"backlinks", "summary", "--target", "example.com"}, "/backlinks/summary/live"},
		{"backlinks list", []string{"backlinks", "list", "--target", "example.com"}, "/backlinks/backlinks/live"},
		{"backlinks referring-domains", []string{"backlinks", "referring-domains", "--target", "example.com"}, "/backlinks/referring_domains/live"},
		{"backlinks anchors", []string{"backlinks", "anchors", "--target", "example.com"}, "/backlinks/anchors/live"},
		{"onpage check", []string{"onpage", "check", "--url", "https://example.com/page"}, "/on_page/instant_pages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, okEnvelope("0.01", `[]`), &got)
			defer srv.Close()
			code, _, se := run(t, srv, tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
			}
			if got.Method != http.MethodPost || got.Path != tc.path {
				t.Errorf("request = %s %s, want POST %s", got.Method, got.Path, tc.path)
			}
			arr := decodeBody(t, got.Body)
			if len(arr) != 1 {
				t.Errorf("task array length = %d, want 1", len(arr))
			}
		})
	}
}

// --- domain/backlinks target + limit fields ---

func TestDomainRankedKeywords_TargetAndLimit(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0.01", `[]`), &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "domain", "ranked-keywords", "--target", "example.com", "--limit", "50"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	task := decodeBody(t, got.Body)[0]
	if task["target"] != "example.com" {
		t.Errorf("target = %v, want example.com", task["target"])
	}
	if task["limit"].(float64) != 50 {
		t.Errorf("limit = %v, want 50", task["limit"])
	}
}

func TestOnpageCheck_URLField(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0.01", `[]`), &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "onpage", "check", "--url", "https://example.com/x"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	task := decodeBody(t, got.Body)[0]
	if task["url"] != "https://example.com/x" {
		t.Errorf("url = %v, want https://example.com/x", task["url"])
	}
}

// --- account is a free GET to user_data ---

func TestAccount_GetUserData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0", `[{"login":"user@example.com","money":{"balance":12.5}}]`), &got)
	defer srv.Close()

	code, stdout, se := run(t, srv, "account")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Method != http.MethodGet || got.Path != "/appendix/user_data" {
		t.Errorf("request = %s %s, want GET /appendix/user_data", got.Method, got.Path)
	}
	if len(got.Body) != 0 {
		t.Errorf("GET body = %q, want empty", got.Body)
	}
	if !strings.Contains(stdout, "user@example.com") {
		t.Errorf("stdout = %q, want the login", stdout)
	}
}

// --- meta locations/languages filter client-side ---

func TestMetaLocations_FilterBySearch(t *testing.T) {
	var got capturedRequest
	body := okEnvelope("0", `[{"location_name":"United States","location_code":2840},{"location_name":"United Kingdom","location_code":2826}]`)
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	code, stdout, se := run(t, srv, "meta", "locations", "--search", "kingdom")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Method != http.MethodGet || got.Path != "/serp/google/locations" {
		t.Errorf("request = %s %s, want GET /serp/google/locations", got.Method, got.Path)
	}
	out := decodeOutput(t, stdout)
	result := out["result"].([]any)
	if len(result) != 1 {
		t.Fatalf("filtered result = %v, want 1 (case-insensitive substring)", result)
	}
	if result[0].(map[string]any)["location_name"] != "United Kingdom" {
		t.Errorf("filtered = %v, want United Kingdom", result[0])
	}
}

// --- (e) HTTP 401 rejects credential ---

func TestHTTP401_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"status_code":40100,"status_message":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for HTTP 401")
	}
	if stderr == "" {
		t.Error("want an error message on stderr")
	}
}

// --- (e) body status_code 40100 (HTTP 200) rejects credential ---

func TestBodyCode40100_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, topErrorEnvelope(40100, "not authorized"), &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "account")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 + credential rejected", result)
	}
}

// --- (f) 40200/40210 give balance guidance, not credential rejection ---

func TestBalanceCodes_GiveGuidance(t *testing.T) {
	for _, code := range []int{40200, 40210} {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, topErrorEnvelope(code, "payment required"), &got)
		result, _, stderr := runResult(t, srv, "serp", "google", "--keyword", "x")
		srv.Close()
		if result.ExitCode != 1 {
			t.Errorf("code %d: exit = %d, want 1", code, result.ExitCode)
		}
		if result.CredentialRejected {
			t.Errorf("code %d: must not reject the credential for a balance error", code)
		}
		if !strings.Contains(strings.ToLower(stderr), "balance") {
			t.Errorf("code %d: stderr = %q, want balance guidance", code, stderr)
		}
	}
}

// --- (d) HTTP 200 with task-level 40501 → apiError exit 1 ---

func TestTaskCode40501_APIErrorExit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, taskErrorEnvelope(40501, "invalid field"), &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "serp", "google", "--keyword", "x")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("a field error must not reject the credential")
	}
	if !strings.Contains(stderr, "40501") || !strings.Contains(stderr, "invalid field") {
		t.Errorf("stderr = %q, want status_code and message", stderr)
	}
}

// --- (g) malformed pair (no colon) → credential error ---

func TestMalformedCredential_NoColon(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0", `[]`), &got)
	defer srv.Close()

	result, _, stderr := runCreds(t, srv, "no-colon-here", "account")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("a malformed login:password pair must reject the credential")
	}
	if got.Method != "" {
		t.Error("no HTTP request should be made with a malformed credential")
	}
	if !strings.Contains(strings.ToLower(stderr), "login:password") {
		t.Errorf("stderr = %q, want a login:password hint", stderr)
	}
}

// --- missing credential env ---

func TestMissingCredential(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvCredentials) {
		t.Errorf("stderr = %q, want mention of %s", errBuf.String(), EnvCredentials)
	}
}

// --- (h) --json error envelope ---

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, taskErrorEnvelope(40501, "invalid field"), &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "serp", "google", "--keyword", "x", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	env := decodeOutput(t, stderr)
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("stderr = %q, want {\"error\":{...}}", stderr)
	}
	if errObj["kind"] != "api" {
		t.Errorf("kind = %v, want api", errObj["kind"])
	}
	if !strings.Contains(errObj["message"].(string), "40501") {
		t.Errorf("message = %v, want the status code", errObj["message"])
	}
}

// --- (i) flag validation: missing required flag → exit 2 ---

func TestMissingRequiredFlag_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0", `[]`), &got)
	defer srv.Close()

	// serp google requires --keyword.
	code, _, _ := run(t, srv, "serp", "google")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for a missing required flag", code)
	}
	if got.Method != "" {
		t.Error("no HTTP request should be made when required flags are missing")
	}
}

// --- (i) unknown subcommand → exit 2 ---

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope("0", `[]`), &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keywords", "nope")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for an unknown subcommand", code)
	}
}
