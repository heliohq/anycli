package ahrefs

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUsage_RequestShapeAndAuth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"limits_and_usage":{"subscription":"Lite"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "usage")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/subscription-info/limits-and-usage" {
		t.Errorf("request = %s %s, want GET /subscription-info/limits-and-usage", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.Query != "" {
		t.Errorf("usage should send no query params, got %q", got.Query)
	}
	if !strings.Contains(stdout, "Lite") {
		t.Errorf("stdout should pass provider JSON through, got %q", stdout)
	}
}

func TestDomainOverview_FansOutToThreeEndpoints(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/site-explorer/domain-rating":   {http.StatusOK, `{"domain_rating":{"domain_rating":72}}`},
		"/site-explorer/backlinks-stats": {http.StatusOK, `{"metrics":{"live":10}}`},
		"/site-explorer/metrics":         {http.StatusOK, `{"metrics":{"org_traffic":500}}`},
	}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "domain", "overview", "--target", "ahrefs.com", "--date", "2026-07-01")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(captured) != 3 {
		t.Fatalf("expected 3 endpoint hits, got %d: %v", len(captured), keys(captured))
	}
	dr := captured["/site-explorer/domain-rating"]
	q := parseQuery(t, dr.Query)
	if q.Get("target") != "ahrefs.com" || q.Get("date") != "2026-07-01" {
		t.Errorf("domain-rating query = %q, want target+date", dr.Query)
	}
	// merged object carries all three sources verbatim.
	var merged map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &merged); err != nil {
		t.Fatalf("stdout not a merged JSON object: %v (%s)", err, stdout)
	}
	for _, k := range []string{"domain_rating", "backlinks_stats", "metrics"} {
		if _, ok := merged[k]; !ok {
			t.Errorf("merged overview missing %q key: %s", k, stdout)
		}
	}
}

func TestDomainOverview_CheapSkipsStatsAndMetrics(t *testing.T) {
	captured := map[string]capturedRequest{}
	routes := map[string]routeHandler{
		"/site-explorer/domain-rating": {http.StatusOK, `{"domain_rating":{"domain_rating":72}}`},
	}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "overview", "--target", "ahrefs.com", "--cheap")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(captured) != 1 {
		t.Fatalf("--cheap should hit only domain-rating, got %v", keys(captured))
	}
	// date defaults to today (non-empty) when omitted.
	if q := parseQuery(t, captured["/site-explorer/domain-rating"].Query); q.Get("date") == "" {
		t.Error("date should default to today when --date omitted")
	}
}

func TestDomainOverview_MissingTargetIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "domain", "overview")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", code)
	}
	if got.Method != "" {
		t.Error("no HTTP request should be made when --target is missing")
	}
	if !strings.Contains(stderr, "--target is required") {
		t.Errorf("stderr = %q, want a --target-required message", stderr)
	}
}

func TestBacklinksList_DefaultSelectAndLimit(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"backlinks":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "backlinks", "list", "--target", "example.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/site-explorer/all-backlinks" {
		t.Errorf("path = %q, want /site-explorer/all-backlinks", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("target") != "example.com" {
		t.Errorf("target = %q", q.Get("target"))
	}
	if q.Get("select") != backlinksDefaultSelect {
		t.Errorf("select = %q, want curated default %q", q.Get("select"), backlinksDefaultSelect)
	}
	if q.Get("limit") != "10" {
		t.Errorf("limit = %q, want default 10 for unit safety", q.Get("limit"))
	}
	if _, ok := q["where"]; ok {
		t.Errorf("where should be omitted when empty, query = %q", got.Query)
	}
}

func TestBacklinksBroken_PathAndWherePassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"backlinks":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "backlinks", "broken", "--target", "example.com",
		"--select", "url_from,url_to", "--where", "domain_rating_source>50", "--limit", "3",
		"--order-by", "traffic:desc", "--offset", "5", "--mode", "domain", "--protocol", "https")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/site-explorer/broken-backlinks" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("select") != "url_from,url_to" {
		t.Errorf("select override = %q", q.Get("select"))
	}
	if q.Get("where") != "domain_rating_source>50" {
		t.Errorf("where passthrough = %q", q.Get("where"))
	}
	if q.Get("order_by") != "traffic:desc" || q.Get("limit") != "3" || q.Get("offset") != "5" {
		t.Errorf("row flags not applied: %q", got.Query)
	}
	if q.Get("mode") != "domain" || q.Get("protocol") != "https" {
		t.Errorf("mode/protocol not applied: %q", got.Query)
	}
}

func TestRefdomains_RequiresTarget(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "refdomains")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--target is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestKeywordsOrganic_SendsTargetDateSelect(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"keywords":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keywords", "organic", "--target", "example.com", "--date", "2026-06-30", "--country", "us")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/site-explorer/organic-keywords" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("target") != "example.com" || q.Get("date") != "2026-06-30" || q.Get("country") != "us" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("select") != organicKeywordsDefaultSelect {
		t.Errorf("select = %q", q.Get("select"))
	}
}

func TestPagesTop_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"pages":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "pages", "top", "--target", "example.com")
	if code != 0 || got.Path != "/site-explorer/top-pages" {
		t.Fatalf("code=%d path=%q", code, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("date") == "" {
		t.Error("top-pages date should default to today")
	}
}

func TestCompetitors_CountryRequired(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"competitors":[]}`, &got)
	defer srv.Close()

	// Missing --country is a usage error.
	code, _, stderr := run(t, srv, "competitors", "--target", "example.com")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--country is required") {
		t.Errorf("stderr = %q", stderr)
	}
	if got.Method != "" {
		t.Error("no request should be made without --country")
	}

	// With --country it goes through.
	code, _, _ = run(t, srv, "competitors", "--target", "example.com", "--country", "us")
	if code != 0 || got.Path != "/site-explorer/organic-competitors" {
		t.Fatalf("code=%d path=%q", code, got.Path)
	}
}

func TestKeywordOverview_KeywordsAndCountry(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"keywords":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keyword", "overview", "--keywords", "seo,backlinks", "--country", "us")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/keywords-explorer/overview" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("keywords") != "seo,backlinks" || q.Get("country") != "us" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("select") != keywordOverviewDefaultSelect {
		t.Errorf("select = %q", q.Get("select"))
	}
	// keywords-explorer rows must not send mode/protocol (no target concept).
	if _, ok := q["mode"]; ok {
		t.Error("keyword overview must not send mode")
	}
}

func TestKeywordIdeas_KindRoutesEndpoint(t *testing.T) {
	cases := map[string]string{
		"matching":    "/keywords-explorer/matching-terms",
		"related":     "/keywords-explorer/related-terms",
		"suggestions": "/keywords-explorer/search-suggestions",
	}
	for kind, path := range cases {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{"keywords":[]}`, &got)
		code, _, _ := run(t, srv, "keyword", "ideas", "--kind", kind, "--keywords", "seo", "--country", "us")
		srv.Close()
		if code != 0 {
			t.Fatalf("kind %s: exit code = %d", kind, code)
		}
		if got.Path != path {
			t.Errorf("kind %s: path = %q, want %q", kind, got.Path, path)
		}
	}
}

func TestKeywordIdeas_BadKindIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "keyword", "ideas", "--kind", "bogus", "--keywords", "seo", "--country", "us")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "matching|related|suggestions") {
		t.Errorf("stderr = %q", stderr)
	}
	if got.Method != "" {
		t.Error("no request for an invalid --kind")
	}
}

func TestKeywordVolumeHistory_NoSelectWithDateRange(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"metrics":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keyword", "volume-history", "--keyword", "seo", "--country", "us",
		"--from", "2025-01-01", "--to", "2026-01-01")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/keywords-explorer/volume-history" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("keyword") != "seo" || q.Get("country") != "us" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("date_from") != "2025-01-01" || q.Get("date_to") != "2026-01-01" {
		t.Errorf("date range = %q", got.Query)
	}
	if _, ok := q["select"]; ok {
		t.Error("volume-history must not send select")
	}
}

func TestSerp_RequiredParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"positions":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "serp", "--keyword", "seo tools", "--country", "us", "--top-positions", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/serp-overview/serp-overview" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("keyword") != "seo tools" || q.Get("country") != "us" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("select") != serpDefaultSelect {
		t.Errorf("select = %q", q.Get("select"))
	}
	if q.Get("top_positions") != "10" {
		t.Errorf("top_positions = %q", q.Get("top_positions"))
	}
}

func TestBatch_PostBodyShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"targets":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "batch", "--targets", "ahrefs.com, example.com", "--select", "url,domain_rating", "--country", "us")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/batch-analysis/batch-analysis" {
		t.Errorf("request = %s %s, want POST /batch-analysis/batch-analysis", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	sel, _ := body["select"].([]any)
	if len(sel) != 2 || sel[0] != "url" || sel[1] != "domain_rating" {
		t.Errorf("select body = %v", body["select"])
	}
	targets, _ := body["targets"].([]any)
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want 2 objects", body["targets"])
	}
	first, _ := targets[0].(map[string]any)
	if first["url"] != "ahrefs.com" || first["mode"] != "subdomains" || first["protocol"] != "both" {
		t.Errorf("first target = %v, want url/mode/protocol with defaults", first)
	}
	if body["country"] != "us" {
		t.Errorf("country = %v", body["country"])
	}
}

func TestBatch_MissingTargetsIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "batch")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--targets is required") {
		t.Errorf("stderr = %q", stderr)
	}
	if got.Method != "" {
		t.Error("no request when --targets missing")
	}
}

func TestAPIError_ExitOneAndMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":"invalid target"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "usage")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (API error)", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("a 400 must not mark the credential rejected")
	}
	if !strings.Contains(stderr, "invalid target") || !strings.Contains(stderr, "400") {
		t.Errorf("stderr = %q, want the HTTP status and Ahrefs message", stderr)
	}
}

func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":"invalid token"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "usage")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("a 401 must classify the credential as rejected")
	}
}

func TestJSONErrorEnvelope_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"error":"rate limited"}`, &got)
	defer srv.Close()

	_, _, stderr := runResult(t, srv, "usage", "--json")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 429 {
		t.Errorf("envelope = %+v, want kind=api status=429", env.Error)
	}
	if !strings.Contains(env.Error.Message, "rate limited") {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestJSONErrorEnvelope_UsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	_, _, stderr := runResult(t, srv, "refdomains", "--json")
	var env struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestMissingToken_ExitOne(t *testing.T) {
	result, _, stderr := runNoToken(t, "usage")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "AHREFS_API_TOKEN is not set") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestMissingToken_JSONEnvelope(t *testing.T) {
	_, _, stderr := runNoToken(t, "usage", "--json")
	if !strings.Contains(stderr, `"error"`) || !strings.Contains(stderr, "AHREFS_API_TOKEN") {
		t.Errorf("stderr = %q, want a JSON envelope naming the token", stderr)
	}
}

func TestUnknownSubcommand_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an unknown subcommand", code)
	}
}

// keys returns the sorted-insensitive key set of a captured map for messages.
func keys(m map[string]capturedRequest) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
