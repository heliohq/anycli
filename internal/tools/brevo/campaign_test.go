package brevo

import (
	"net/http"
	"testing"
)

func TestCampaignList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"campaigns":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "list", "--type", "classic", "--status", "sent", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/emailCampaigns" {
		t.Errorf("request = %s %s, want GET /emailCampaigns", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("type") != "classic" || q.Get("status") != "sent" || q.Get("limit") != "10" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestCampaignGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":3}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "get", "--id", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/emailCampaigns/3" {
		t.Errorf("request = %s %s, want GET /emailCampaigns/3", got.Method, got.Path)
	}
}

func TestCampaignCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":11}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "create",
		"--name", "Promo", "--subject", "Sale", "--html", "<p>Buy</p>",
		"--sender-email", "n@myco.com", "--sender-name", "MyCo",
		"--list-ids", "9", "--list-ids", "10", "--scheduled-at", "2030-01-01T10:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/emailCampaigns" {
		t.Errorf("request = %s %s, want POST /emailCampaigns", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Promo" || body["subject"] != "Sale" || body["htmlContent"] != "<p>Buy</p>" {
		t.Errorf("body = %v", body)
	}
	sender, ok := body["sender"].(map[string]any)
	if !ok || sender["email"] != "n@myco.com" || sender["name"] != "MyCo" {
		t.Errorf("sender = %v", body["sender"])
	}
	recipients, ok := body["recipients"].(map[string]any)
	if !ok {
		t.Fatalf("recipients = %v, want object", body["recipients"])
	}
	listIDs, ok := recipients["listIds"].([]any)
	if !ok || len(listIDs) != 2 || listIDs[0] != float64(9) {
		t.Errorf("recipients.listIds = %v, want [9,10]", recipients["listIds"])
	}
	if body["scheduledAt"] != "2030-01-01T10:00:00Z" {
		t.Errorf("scheduledAt = %v", body["scheduledAt"])
	}
}
