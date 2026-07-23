package sproutsocial

import (
	"strings"
	"testing"
)

// --- discovery / metadata ---

func TestMetadataClient_NoCustomerIDInPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[{"customer_id":687751,"name":"My Business"}]}`, &got, nil)
	defer srv.Close()

	exit, out, errOut := run(t, srv, "metadata", "client")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errOut)
	}
	if got.Path != "/v1/metadata/client" {
		t.Errorf("path = %q, want /v1/metadata/client (no customer id)", got.Path)
	}
	if got.Method != "GET" {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("auth = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(out, `"customer_id":687751`) {
		t.Errorf("stdout should pass through the envelope, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("stdout should be newline-terminated, got %q", out)
	}
}

func TestMetadataProfiles_UsesInjectedCustomerID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	exit, _, errOut := run(t, srv, "metadata", "profiles")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errOut)
	}
	if got.Path != "/v1/687751/metadata/customer" {
		t.Errorf("path = %q, want /v1/687751/metadata/customer", got.Path)
	}
}

func TestMetadataResource_Paths(t *testing.T) {
	cases := map[string]string{
		"tags":   "/v1/687751/metadata/customer/tags",
		"groups": "/v1/687751/metadata/customer/groups",
		"users":  "/v1/687751/metadata/customer/users",
		"topics": "/v1/687751/metadata/customer/topics",
		"teams":  "/v1/687751/metadata/customer/teams",
		"queues": "/v1/687751/metadata/customer/queues",
	}
	for sub, wantPath := range cases {
		t.Run(sub, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, 200, `{"data":[]}`, &got, nil)
			defer srv.Close()
			if exit, _, e := run(t, srv, "metadata", sub); exit != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
			}
			if got.Path != wantPath {
				t.Errorf("path = %q, want %q", got.Path, wantPath)
			}
		})
	}
}

// --- customer id override + resolution ---

func TestCustomerIDFlagOverridesEnv(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	if exit, _, e := run(t, srv, "metadata", "profiles", "--customer-id", "999"); exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	if got.Path != "/v1/999/metadata/customer" {
		t.Errorf("path = %q, want the overridden id /v1/999/metadata/customer", got.Path)
	}
}

func TestMissingCustomerID_IsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	// No customer id in env and no --customer-id.
	exit, _, errOut := runEnv(t, srv, map[string]string{EnvToken: "tok-123"}, "metadata", "profiles")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should have been made, got %s %s", got.Method, got.Path)
	}
	if !strings.Contains(errOut, "customer id") {
		t.Errorf("stderr should explain the missing customer id, got %q", errOut)
	}
}

// --- analytics POST body assembly ---

func TestAnalyticsPosts_BuildsFilterMetricBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[],"paging":{"current_page":1,"total_pages":1}}`, &got, nil)
	defer srv.Close()

	exit, _, errOut := run(t, srv, "analytics", "posts",
		"--filter", "created_time.in(2026-01-01...2026-02-01)",
		"--filter", "customer_profile_id.eq(1,2)",
		"--metric", "impressions",
		"--metric", "reactions",
		"--fields", "text,internal.tags.id",
		"--page", "2",
		"--limit", "50",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errOut)
	}
	if got.Method != "POST" || got.Path != "/v1/687751/analytics/posts" {
		t.Fatalf("request = %s %s, want POST /v1/687751/analytics/posts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	filters, ok := body["filters"].([]any)
	if !ok || len(filters) != 2 || filters[0] != "created_time.in(2026-01-01...2026-02-01)" {
		t.Errorf("filters = %v, want the two DSL clauses as an array", body["filters"])
	}
	metrics, ok := body["metrics"].([]any)
	if !ok || len(metrics) != 2 || metrics[0] != "impressions" {
		t.Errorf("metrics = %v, want [impressions reactions]", body["metrics"])
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 2 || fields[1] != "internal.tags.id" {
		t.Errorf("fields = %v, want the comma-split array", body["fields"])
	}
	if body["page"] != float64(2) {
		t.Errorf("page = %v, want 2", body["page"])
	}
	if body["limit"] != float64(50) {
		t.Errorf("limit = %v, want 50", body["limit"])
	}
}

func TestBodyFlagOverridesAssembledBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	exit, _, errOut := run(t, srv, "analytics", "profiles",
		"--filter", "ignored.eq(1)",
		"--body", `{"filters":["customer_profile_id.eq(42)"],"metrics":["followers_count"]}`,
	)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errOut)
	}
	body := decodeBody(t, got.Body)
	if _, hasPage := body["page"]; hasPage {
		t.Errorf("--body should be verbatim; unexpected assembled keys: %v", body)
	}
	filters, ok := body["filters"].([]any)
	if !ok || len(filters) != 1 || filters[0] != "customer_profile_id.eq(42)" {
		t.Errorf("filters = %v, want the verbatim --body filters", body["filters"])
	}
	if _, ok := body["metrics"].([]any); !ok {
		t.Errorf("metrics missing; --body not passed through: %v", body)
	}
}

func TestBodyFlag_InvalidJSON_IsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got, nil)
	defer srv.Close()

	exit, _, errOut := run(t, srv, "messages", "list", "--body", `{not json`)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should have been made on a parse error")
	}
	if !strings.Contains(errOut, "--body") {
		t.Errorf("stderr should name --body, got %q", errOut)
	}
}

// --- inbox + cases paths ---

func TestMessagesList_PathAndCursor(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[],"paging":{"next_cursor":"abc"}}`, &got, nil)
	defer srv.Close()

	exit, _, e := run(t, srv, "messages", "list",
		"--filter", "created_time.in(2026-01-01...2026-01-02)",
		"--page-cursor", "PREV", "--limit", "100")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	if got.Method != "POST" || got.Path != "/v1/687751/messages" {
		t.Fatalf("request = %s %s, want POST /v1/687751/messages", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["page_cursor"] != "PREV" {
		t.Errorf("page_cursor = %v, want PREV", body["page_cursor"])
	}
	if body["limit"] != float64(100) {
		t.Errorf("limit = %v, want 100", body["limit"])
	}
}

func TestCasesFilter_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	if exit, _, e := run(t, srv, "cases", "filter", "--filter", "case_id.eq(5)"); exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	if got.Method != "POST" || got.Path != "/v1/687751/cases/filter" {
		t.Fatalf("request = %s %s, want POST /v1/687751/cases/filter", got.Method, got.Path)
	}
}

// --- publishing ---

func TestPublishingCreate_ForcesDraftAndCoercesIDs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	exit, _, e := run(t, srv, "publishing", "create",
		"--group-id", "12", "--profile-id", "100", "--profile-id", "200", "--text", "hello")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	if got.Method != "POST" || got.Path != "/v1/687751/publishing/posts" {
		t.Fatalf("request = %s %s, want POST /v1/687751/publishing/posts", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["is_draft"] != true {
		t.Errorf("is_draft = %v, want true (only draft is supported)", body["is_draft"])
	}
	if body["group_id"] != float64(12) {
		t.Errorf("group_id = %v, want numeric 12", body["group_id"])
	}
	profiles, ok := body["customer_profile_ids"].([]any)
	if !ok || len(profiles) != 2 || profiles[0] != float64(100) {
		t.Errorf("customer_profile_ids = %v, want numeric [100 200]", body["customer_profile_ids"])
	}
	if body["text"] != "hello" {
		t.Errorf("text = %v, want hello", body["text"])
	}
}

func TestPublishingCreate_MissingGroupID_IsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got, nil)
	defer srv.Close()

	exit, _, errOut := run(t, srv, "publishing", "create", "--profile-id", "100")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected on a usage error")
	}
	if !strings.Contains(errOut, "group-id") {
		t.Errorf("stderr should name --group-id, got %q", errOut)
	}
}

func TestPublishingCreate_BodyOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got, nil)
	defer srv.Close()

	exit, _, e := run(t, srv, "publishing", "create",
		"--body", `{"group_id":9,"customer_profile_ids":[7],"is_draft":true,"media":[{"url":"x"}]}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["media"]; !ok {
		t.Errorf("--body should pass media through verbatim, got %v", body)
	}
}

func TestPublishingGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{}}`, &got, nil)
	defer srv.Close()

	if exit, _, e := run(t, srv, "publishing", "get", "abc123"); exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, e)
	}
	if got.Method != "GET" || got.Path != "/v1/687751/publishing/posts/abc123" {
		t.Fatalf("request = %s %s, want GET /v1/687751/publishing/posts/abc123", got.Method, got.Path)
	}
}

// --- error contract ---

func TestAPIError_Non2xx_Exit1WithRequestID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 400, `{"error":"bad filter"}`, &got, map[string]string{"X-Sprout-Request-ID": "req-9"})
	defer srv.Close()

	exit, _, errOut := run(t, srv, "cases", "filter", "--filter", "bad")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (api)", exit)
	}
	if !strings.Contains(errOut, "HTTP 400") || !strings.Contains(errOut, "bad filter") {
		t.Errorf("stderr should carry the status + Sprout error, got %q", errOut)
	}
}

func TestAPIError_JSONMode_EnvelopeWithRequestID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 403, `{"error":"forbidden"}`, &got, map[string]string{"X-Sprout-Request-ID": "req-42"})
	defer srv.Close()

	exit, _, errOut := run(t, srv, "metadata", "profiles", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (api)", exit)
	}
	for _, want := range []string{`"kind":"api"`, `"status":403`, `"request_id":"req-42"`} {
		if !strings.Contains(errOut, want) {
			t.Errorf("json error envelope missing %s, got %q", want, errOut)
		}
	}
}

func TestUnauthorized_Exit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"error":"invalid token"}`, &got, nil)
	defer srv.Close()

	exit, _, _ := run(t, srv, "metadata", "client")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (credential rejected → api failure)", exit)
	}
}

func TestMissingToken_Exit1(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{}, nil)
	defer srv.Close()

	exit, _, errOut := runEnv(t, srv, map[string]string{}, "metadata", "client")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(errOut, "SPROUT_SOCIAL_TOKEN") {
		t.Errorf("stderr should name the missing token env, got %q", errOut)
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{}, nil)
	defer srv.Close()

	exit, _, _ := run(t, srv, "metadata", "nope")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
}
