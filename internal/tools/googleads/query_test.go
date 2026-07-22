package googleads

import (
	"net/http"
	"strings"
	"testing"
)

func TestQuery_SearchPostsGAQLAndPassesThrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"campaign":{"id":"1"}}],"nextPageToken":"abc"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "query", "--customer-id", "123-456-7890",
		"--gaql", "SELECT campaign.id FROM campaign", "--page-size", "50")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/customers/1234567890/googleAds:search" {
		t.Errorf("request = %s %s, want POST /customers/1234567890/googleAds:search", got.Method, got.Path)
	}
	if got.Auth != "Bearer user-tok" || got.DeveloperToken != "dev-tok" {
		t.Errorf("auth headers = %q / %q", got.Auth, got.DeveloperToken)
	}
	body := decodeBody(t, got.Body)
	if body["query"] != "SELECT campaign.id FROM campaign" {
		t.Errorf("query body = %v", body["query"])
	}
	if body["pageSize"].(float64) != 50 {
		t.Errorf("pageSize = %v, want 50", body["pageSize"])
	}
	if !strings.Contains(stdout, `"nextPageToken":"abc"`) {
		t.Errorf("stdout = %q, want verbatim search passthrough", stdout)
	}
}

func TestQuery_StreamFlattensJSONArray(t *testing.T) {
	var got capturedRequest
	// searchStream returns a JSON ARRAY of chunks — the documented quirk.
	stream := `[{"results":[{"campaign":{"id":"1"}}],"fieldMask":"campaign.id"},` +
		`{"results":[{"campaign":{"id":"2"}}],"fieldMask":"campaign.id"}]`
	srv := newServer(t, http.StatusOK, stream, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "query", "--customer-id", "1234567890",
		"--gaql", "SELECT campaign.id FROM campaign", "--stream")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/customers/1234567890/googleAds:searchStream" {
		t.Errorf("path = %q, want searchStream", got.Path)
	}
	// The streamed array must be flattened into ONE object with a single
	// results array; the array shape must never leak to the caller.
	if strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("stdout is still a JSON array (quirk leaked): %q", stdout)
	}
	if !strings.Contains(stdout, `"results":[`) {
		t.Errorf("stdout = %q, want a flattened results object", stdout)
	}
	if !strings.Contains(stdout, `{"campaign":{"id":"1"}}`) || !strings.Contains(stdout, `{"campaign":{"id":"2"}}`) {
		t.Errorf("stdout = %q, want both chunks' rows merged", stdout)
	}
	if !strings.Contains(stdout, `"fieldMask":"campaign.id"`) {
		t.Errorf("stdout = %q, want the field mask preserved", stdout)
	}
}

func TestQuery_MissingGAQLIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// cobra enforces --gaql required → exit 2, no request sent.
	code, _, stderr := run(t, srv, "query", "--customer-id", "1234567890")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("a request was sent for an invalid invocation: %s %s", got.Method, got.Path)
	}
	if stderr == "" {
		t.Error("expected a usage error on stderr")
	}
}

func TestQuery_BadCustomerIDIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "query", "--customer-id", "not-a-number", "--gaql", "SELECT campaign.id FROM campaign")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("a request was sent for a bad customer id: %s %s", got.Method, got.Path)
	}
}

func TestReport_BuildsGAQLAndStreams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"results":[],"fieldMask":"campaign.id"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "report", "--customer-id", "1234567890", "--resource", "campaign", "--date-range", "LAST_7_DAYS")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/customers/1234567890/googleAds:searchStream" {
		t.Errorf("path = %q, want searchStream (report is bulk)", got.Path)
	}
	body := decodeBody(t, got.Body)
	gaql, _ := body["query"].(string)
	for _, want := range []string{
		"SELECT ", "FROM campaign", "WHERE segments.date DURING LAST_7_DAYS",
		"campaign.id", "campaign.name", "campaign.status",
		"metrics.impressions", "metrics.clicks", "metrics.cost_micros", "metrics.conversions",
	} {
		if !strings.Contains(gaql, want) {
			t.Errorf("composed GAQL %q missing %q", gaql, want)
		}
	}
}

func TestReport_CustomMetricsAndSegments(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "report", "--customer-id", "1234567890", "--resource", "keyword",
		"--metrics", "metrics.clicks,metrics.ctr", "--segments", "segments.date")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	gaql, _ := body["query"].(string)
	if !strings.Contains(gaql, "FROM keyword_view") {
		t.Errorf("keyword resource should map to keyword_view: %q", gaql)
	}
	if !strings.Contains(gaql, "metrics.ctr") || !strings.Contains(gaql, "segments.date") {
		t.Errorf("custom metrics/segments not composed: %q", gaql)
	}
	if strings.Contains(gaql, "metrics.impressions") {
		t.Errorf("default metrics should be replaced by --metrics: %q", gaql)
	}
}

func TestReport_InvalidResourceAndDateRangeAreUsageErrors(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "report", "--customer-id", "1234567890", "--resource", "banana")
	if code != 2 {
		t.Errorf("bad resource exit = %d, want 2", code)
	}
	code, _, _ = run(t, srv, "report", "--customer-id", "1234567890", "--resource", "campaign", "--date-range", "SINCE_FOREVER")
	if code != 2 {
		t.Errorf("bad date-range exit = %d, want 2", code)
	}
	code, _, _ = run(t, srv, "report", "--customer-id", "1234567890", "--resource", "campaign", "--metrics", "DROP TABLE")
	if code != 2 {
		t.Errorf("injection-y metric exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("a request was sent for an invalid report: %s %s", got.Method, got.Path)
	}
}

func TestQuery_LoginCustomerIDHeaderInjected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "query", "--customer-id", "1234567890",
		"--gaql", "SELECT campaign.id FROM campaign", "--login-customer-id", "555-000-1111")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.LoginCustomerID != "555-000-1111" {
		t.Errorf("login-customer-id header = %q, want the flag value verbatim", got.LoginCustomerID)
	}
}
