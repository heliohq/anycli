package mailerlite

import (
	"net/http"
	"testing"
)

func TestCampaignList_Filters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[],"meta":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "list", "--status", "sent", "--type", "regular", "--limit", "5", "--page", "2")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/campaigns" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("filter[status]") != "sent" || q.Get("filter[type]") != "regular" {
		t.Errorf("filters = %q", got.Query)
	}
	if q.Get("limit") != "5" || q.Get("page") != "2" {
		t.Errorf("pagination = %q", got.Query)
	}
}

func TestCampaignSchedule_DataBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "schedule", "77", "--data", `{"delivery":"instant"}`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/campaigns/77/schedule" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["delivery"] != "instant" {
		t.Errorf("body = %v", body)
	}
}

func TestCampaignSchedule_InvalidJSON_IsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "campaign", "schedule", "77", "--data", `{not json`)
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
}

func TestCampaignCancel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "campaign", "cancel", "77"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/campaigns/77/cancel" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestCampaignReport(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "campaign", "report", "77"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/campaigns/77/reports/subscriber-activity" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestFieldCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"data":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "field", "create", "--name", "Company", "--type", "text"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/fields" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Company" || body["type"] != "text" {
		t.Errorf("body = %v", body)
	}
}

func TestSegmentSubscribers(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "segment", "subscribers", "12"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/segments/12/subscribers" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestFormList_ValidType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "list", "popup"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/forms/popup" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestFormList_InvalidType_IsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "form", "list", "banner")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should be made for an invalid type, got %s %s", got.Method, got.Path)
	}
}

func TestWebhookCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "webhook", "create", "--url", "https://example.com/hook", "--events", "subscriber.created,subscriber.updated", "--name", "hook1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/webhooks" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["url"] != "https://example.com/hook" || body["name"] != "hook1" {
		t.Errorf("body = %v", body)
	}
	events, _ := body["events"].([]any)
	if len(events) != 2 || events[0] != "subscriber.created" {
		t.Errorf("events = %v", body["events"])
	}
}

func TestAutomationActivity(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "automation", "activity", "88"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/automations/88/activity" {
		t.Errorf("path = %q", got.Path)
	}
}
