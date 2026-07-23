package googleads

import (
	"net/http"
	"strings"
	"testing"
)

func TestCampaignsList_ComposesGAQLWithStatusFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaigns", "list", "--customer-id", "1234567890", "--status", "enabled")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/customers/1234567890/googleAds:search" {
		t.Errorf("path = %q, want googleAds:search", got.Path)
	}
	body := decodeBody(t, got.Body)
	gaql, _ := body["query"].(string)
	if !strings.Contains(gaql, "FROM campaign") || !strings.Contains(gaql, "WHERE campaign.status = 'ENABLED'") {
		t.Errorf("GAQL = %q, want a campaign list with an uppercased status filter", gaql)
	}
}

func TestCampaignSetStatus_MutateBodyAndMask(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"resourceName":"customers/1234567890/campaigns/55"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "campaign", "set-status", "--customer-id", "1234567890", "--id", "55", "--status", "paused")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/customers/1234567890/campaigns:mutate" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	ops, ok := body["operations"].([]any)
	if !ok || len(ops) != 1 {
		t.Fatalf("operations = %v", body["operations"])
	}
	op := ops[0].(map[string]any)
	if op["updateMask"] != "status" {
		t.Errorf("updateMask = %v, want status", op["updateMask"])
	}
	update := op["update"].(map[string]any)
	if update["status"] != "PAUSED" {
		t.Errorf("status = %v, want PAUSED (uppercased)", update["status"])
	}
	if update["resourceName"] != "customers/1234567890/campaigns/55" {
		t.Errorf("resourceName = %v", update["resourceName"])
	}
	if !strings.Contains(stdout, `"resourceName"`) {
		t.Errorf("stdout = %q, want mutate passthrough", stdout)
	}
}

func TestCampaignSetStatus_RejectsRemoved(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "set-status", "--customer-id", "1234567890", "--id", "55", "--status", "REMOVED")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (REMOVED not allowed)", code)
	}
	if got.Method != "" {
		t.Errorf("a mutate was sent for a disallowed status: %s %s", got.Method, got.Path)
	}
}

func TestBudgetSet_MutateAmountMicrosAsString(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"resourceName":"customers/1234567890/campaignBudgets/9"}]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "budget", "set", "--customer-id", "1234567890", "--id", "9", "--amount-micros", "5000000")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/customers/1234567890/campaignBudgets:mutate" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	op := body["operations"].([]any)[0].(map[string]any)
	if op["updateMask"] != "amount_micros" {
		t.Errorf("updateMask = %v, want amount_micros", op["updateMask"])
	}
	update := op["update"].(map[string]any)
	// Google encodes int64 fields as JSON strings; amountMicros must be a string.
	if update["amountMicros"] != "5000000" {
		t.Errorf("amountMicros = %#v, want the string \"5000000\"", update["amountMicros"])
	}
}

func TestBudgetSet_RejectsNonPositiveAmount(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "budget", "set", "--customer-id", "1234567890", "--id", "9", "--amount-micros", "0")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("a mutate was sent for a non-positive amount: %s %s", got.Method, got.Path)
	}
}
