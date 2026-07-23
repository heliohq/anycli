package metaads

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAccountsListRequestShape(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"act_1","name":"Test"}]}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "accounts", "list", "--limit", "25")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", captured.Method)
	}
	if captured.Path != "/"+GraphVersion+"/me/adaccounts" {
		t.Fatalf("path = %q", captured.Path)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("limit") != "25" {
		t.Fatalf("limit = %q, want 25", q.Get("limit"))
	}
	if q.Get("fields") == "" {
		t.Fatal("fields not sent")
	}
	if !strings.Contains(stdout, "act_1") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestCampaignListBuildsAccountEdgeAndStatusFilter(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "campaign", "list", "--account", "act_123", "--status", "ACTIVE", "--after", "CURSOR")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/act_123/campaigns" {
		t.Fatalf("path = %q", captured.Path)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("effective_status") != `["ACTIVE"]` {
		t.Fatalf("effective_status = %q", q.Get("effective_status"))
	}
	if q.Get("after") != "CURSOR" {
		t.Fatalf("after = %q", q.Get("after"))
	}
}

func TestCampaignListRequiresAccount(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "campaign", "list")
	if code == 0 {
		t.Fatal("missing --account returned exit 0")
	}
	if !strings.Contains(stderr, "act_<number>") {
		t.Fatalf("stderr = %q, want account format hint", stderr)
	}
}

func TestCampaignListRejectsBareNumericAccount(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "campaign", "list", "--account", "123")
	if code == 0 {
		t.Fatal("bare numeric --account returned exit 0")
	}
	if !strings.Contains(stderr, "act_<number>") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCampaignGet(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"999","name":"C"}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "campaign", "get", "999")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/999" {
		t.Fatalf("path = %q", captured.Path)
	}
}

func TestCampaignCreatePostsForm(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"777"}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"campaign", "create", "--account", "act_5", "--name", "Launch", "--objective", "OUTCOME_TRAFFIC")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", captured.Method)
	}
	if captured.Path != "/"+GraphVersion+"/act_5/campaigns" {
		t.Fatalf("path = %q", captured.Path)
	}
	form, _ := url.ParseQuery(captured.Body)
	if form.Get("name") != "Launch" || form.Get("objective") != "OUTCOME_TRAFFIC" {
		t.Fatalf("form = %q", captured.Body)
	}
	if form.Get("status") != "PAUSED" {
		t.Fatalf("default status = %q, want PAUSED", form.Get("status"))
	}
	if form.Get("special_ad_categories") != "[]" {
		t.Fatalf("special_ad_categories = %q, want []", form.Get("special_ad_categories"))
	}
}

func TestCampaignUpdateStatusAndBudget(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"success":true}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"campaign", "update", "42", "--status", "PAUSED", "--daily-budget", "5000")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/42" {
		t.Fatalf("path = %q", captured.Path)
	}
	form, _ := url.ParseQuery(captured.Body)
	if form.Get("status") != "PAUSED" || form.Get("daily_budget") != "5000" {
		t.Fatalf("form = %q", captured.Body)
	}
}

func TestCampaignUpdateRequiresAMutation(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "campaign", "update", "42")
	if code == 0 {
		t.Fatal("empty update returned exit 0")
	}
	if !strings.Contains(stderr, "nothing to update") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUpdateRejectsInvalidStatus(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "adset", "update", "42", "--status", "RUNNING")
	if code == 0 {
		t.Fatal("invalid status returned exit 0")
	}
	if !strings.Contains(stderr, "ACTIVE, PAUSED, ARCHIVED") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAdSetListWithCampaignFilter(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "adset", "list", "--account", "act_9", "--campaign", "100")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/act_9/adsets" {
		t.Fatalf("path = %q", captured.Path)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("campaign_id") != "100" {
		t.Fatalf("campaign_id = %q", q.Get("campaign_id"))
	}
}

func TestAdListWithAdSetFilter(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "ad", "list", "--account", "act_9", "--adset", "200")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("adset_id") != "200" {
		t.Fatalf("adset_id = %q", q.Get("adset_id"))
	}
}

func TestCreativeList(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "creative", "list", "--account", "act_9")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/act_9/adcreatives" {
		t.Fatalf("path = %q", captured.Path)
	}
}

func TestInsightsAccountLevelDefaultPreset(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "insights", "--account", "act_9", "--level", "campaign")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/act_9/insights" {
		t.Fatalf("path = %q", captured.Path)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("level") != "campaign" {
		t.Fatalf("level = %q", q.Get("level"))
	}
	if q.Get("date_preset") != "last_30d" {
		t.Fatalf("date_preset = %q, want default last_30d", q.Get("date_preset"))
	}
}

func TestInsightsObjectWithTimeRangeOmitsPreset(t *testing.T) {
	var captured capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	tr := `{"since":"2026-01-01","until":"2026-01-31"}`
	code, _, stderr := run(t, server, fullEnv(), "insights", "--object", "555", "--time-range", tr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if captured.Path != "/"+GraphVersion+"/555/insights" {
		t.Fatalf("path = %q", captured.Path)
	}
	q, _ := url.ParseQuery(captured.RawQuery)
	if q.Get("time_range") != tr {
		t.Fatalf("time_range = %q", q.Get("time_range"))
	}
	if q.Get("date_preset") != "" {
		t.Fatalf("date_preset should be omitted when time_range set, got %q", q.Get("date_preset"))
	}
}

func TestInsightsRequiresExactlyOneTarget(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "insights")
	if code == 0 {
		t.Fatal("no target returned exit 0")
	}
	if !strings.Contains(stderr, "exactly one of --account or --object") {
		t.Fatalf("stderr = %q", stderr)
	}

	code, _, stderr = run(t, nil, fullEnv(), "insights", "--account", "act_1", "--object", "5")
	if code == 0 {
		t.Fatal("both targets returned exit 0")
	}
	if !strings.Contains(stderr, "exactly one of --account or --object") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestInsightsRejectsInvalidLevel(t *testing.T) {
	code, _, stderr := run(t, nil, fullEnv(), "insights", "--account", "act_1", "--level", "banner")
	if code == 0 {
		t.Fatal("invalid level returned exit 0")
	}
	if !strings.Contains(stderr, "account, campaign, adset, ad") {
		t.Fatalf("stderr = %q", stderr)
	}
}
