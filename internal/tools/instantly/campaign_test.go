package instantly

import (
	"net/http"
	"testing"
)

func TestCampaignListMapsPaginationAndFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "campaign", "list", "--limit", "25", "--starting-after", "cur1", "--status", "1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/campaigns" {
		t.Fatalf("got %s %s, want GET /campaigns", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "25" || q.Get("starting_after") != "cur1" || q.Get("status") != "1" {
		t.Fatalf("query = %v", q)
	}
}

func TestCampaignListOmitsUnsetFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	run(t, srv, "campaign", "list")
	q := parseQuery(t, got.Query)
	if _, ok := q["limit"]; ok {
		t.Fatalf("limit should be omitted when unset, query=%v", q)
	}
	if _, ok := q["status"]; ok {
		t.Fatalf("status should be omitted when unset, query=%v", q)
	}
}

func TestCampaignGetPathEscapesID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"c1"}`, &got)
	defer srv.Close()

	run(t, srv, "campaign", "get", "--id", "c 1")
	if got.Method != http.MethodGet || got.Path != "/campaigns/c 1" {
		t.Fatalf("got %s %s, want GET /campaigns/c 1 (server-decoded)", got.Method, got.Path)
	}
}

func TestCampaignGetRequiresID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "campaign", "get")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for missing required --id", exit)
	}
}

func TestCampaignCreateMergesDataAndNameOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"c2"}`, &got)
	defer srv.Close()

	run(t, srv, "campaign", "create", "--data", `{"name":"old","schedule":{"x":1}}`, "--name", "new")
	if got.Method != http.MethodPost || got.Path != "/campaigns" {
		t.Fatalf("got %s %s, want POST /campaigns", got.Method, got.Path)
	}
	if got.CType != "application/json" {
		t.Fatalf("Content-Type = %q", got.CType)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "new" {
		t.Fatalf("name = %v, want flag override 'new'", body["name"])
	}
	if _, ok := body["schedule"]; !ok {
		t.Fatalf("schedule from --data should be preserved: %v", body)
	}
}

func TestCampaignActivateAndPause(t *testing.T) {
	for _, tc := range []struct {
		action string
		path   string
	}{
		{"activate", "/campaigns/c1/activate"},
		{"pause", "/campaigns/c1/pause"},
	} {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
		exit, _, _ := run(t, srv, "campaign", tc.action, "--id", "c1")
		srv.Close()
		if exit != 0 {
			t.Fatalf("%s exit = %d", tc.action, exit)
		}
		if got.Method != http.MethodPost || got.Path != tc.path {
			t.Fatalf("%s got %s %s, want POST %s", tc.action, got.Method, got.Path, tc.path)
		}
		if len(got.Body) != 0 {
			t.Fatalf("%s should send no body, got %q", tc.action, got.Body)
		}
	}
}

func TestCampaignAnalyticsVariantsHitCorrectPaths(t *testing.T) {
	for _, tc := range []struct {
		args []string
		path string
	}{
		{[]string{"campaign", "analytics", "--id", "c1"}, "/campaigns/analytics"},
		{[]string{"campaign", "analytics-overview", "--ids", "c1,c2"}, "/campaigns/analytics/overview"},
		{[]string{"campaign", "analytics-daily", "--campaign-id", "c1"}, "/campaigns/analytics/daily"},
		{[]string{"campaign", "analytics-steps", "--campaign-id", "c1"}, "/campaigns/analytics/steps"},
	} {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{}`, &got)
		exit, _, _ := run(t, srv, tc.args...)
		srv.Close()
		if exit != 0 {
			t.Fatalf("%v exit = %d", tc.args, exit)
		}
		if got.Path != tc.path {
			t.Fatalf("%v got path %s, want %s", tc.args, got.Path, tc.path)
		}
	}
}

func TestCampaignAnalyticsDailyUsesCampaignIDParam(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "campaign", "analytics-daily", "--campaign-id", "c9", "--start-date", "2026-01-01")
	q := parseQuery(t, got.Query)
	if q.Get("campaign_id") != "c9" || q.Get("start_date") != "2026-01-01" {
		t.Fatalf("query = %v, want campaign_id=c9 start_date=2026-01-01", q)
	}
}
