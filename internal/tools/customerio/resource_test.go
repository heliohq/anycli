package customerio

import (
	"net/http"
	"reflect"
	"testing"
)

func TestPersonSearch_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"customers":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "person", "search", "--email", "jane@example.com", "--limit", "5")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/customers" {
		t.Errorf("got %s %s, want GET /v1/customers", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("email") != "jane@example.com" {
		t.Errorf("email query = %q", q.Get("email"))
	}
	if q.Get("limit") != "5" {
		t.Errorf("limit query = %q, want 5", q.Get("limit"))
	}
}

func TestPersonSearch_ByFilterPostsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"identifiers":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "person", "search", "--filter", `{"segment":{"id":3}}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/customers" {
		t.Errorf("got %s %s, want POST /v1/customers", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	filter, ok := body["filter"].(map[string]any)
	if !ok {
		t.Fatalf("body.filter missing/wrong type: %v", body)
	}
	seg, ok := filter["segment"].(map[string]any)
	if !ok || seg["id"].(float64) != 3 {
		t.Errorf("filter.segment = %v, want {id:3}", filter["segment"])
	}
}

func TestPersonSearch_RequiresExactlyOneSelector(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// Neither selector.
	if exit, _, _ := run(t, srv, "person", "search"); exit != 2 {
		t.Errorf("no-selector exit = %d, want 2", exit)
	}
	// Both selectors.
	if exit, _, _ := run(t, srv, "person", "search", "--email", "a@b.co", "--filter", "{}"); exit != 2 {
		t.Errorf("both-selectors exit = %d, want 2", exit)
	}
}

func TestPersonGet_IDTypeQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"customer":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "person", "get", "--id", "jane@example.com", "--id-type", "email")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/v1/customers/jane@example.com/attributes" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("id_type") != "email" {
		t.Errorf("id_type = %q, want email", q.Get("id_type"))
	}
}

func TestPersonGet_DefaultIDTypeOmitsQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "person", "get", "--id", "42"); exit != 0 {
		t.Fatalf("exit != 0")
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty for default id-type", got.Query)
	}
}

func TestCampaignMetrics_LinksAndParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"metric":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "campaign", "metrics", "--id", "5", "--links", "--period", "days", "--steps", "7", "--type", "delivered")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/v1/campaigns/5/metrics/links" {
		t.Errorf("path = %q, want /v1/campaigns/5/metrics/links", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("period") != "days" || q.Get("steps") != "7" || q.Get("type") != "delivered" {
		t.Errorf("query = %q, want period=days steps=7 type=delivered", got.Query)
	}
}

func TestCampaignMetrics_JourneyPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "campaign", "metrics", "--id", "5", "--journey"); exit != 0 {
		t.Fatalf("exit != 0")
	}
	if got.Path != "/v1/campaigns/5/journey_metrics" {
		t.Errorf("path = %q, want /v1/campaigns/5/journey_metrics", got.Path)
	}
}

func TestBroadcastTrigger_AudienceEmails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":88}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "broadcast", "trigger", "--id", "12",
		"--data", `{"promo":"x"}`, "--emails", "a@b.co,c@d.co")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/campaigns/12/triggers" {
		t.Errorf("got %s %s, want POST /v1/campaigns/12/triggers", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	emails, ok := body["emails"].([]any)
	if !ok || len(emails) != 2 || emails[0] != "a@b.co" {
		t.Errorf("emails = %v, want [a@b.co c@d.co]", body["emails"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["promo"] != "x" {
		t.Errorf("data = %v, want {promo:x}", body["data"])
	}
}

func TestBroadcastTrigger_RejectsMultipleAudiences(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "broadcast", "trigger", "--id", "12",
		"--emails", "a@b.co", "--ids", "1")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for conflicting audience selectors", exit)
	}
}

func TestSegmentCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"segment":{"id":9}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "segment", "create", "--name", "VIPs", "--description", "top accounts")
	if exit != 0 {
		t.Fatalf("exit != 0")
	}
	body := decodeBody(t, got.Body)
	seg, ok := body["segment"].(map[string]any)
	if !ok || seg["name"] != "VIPs" || seg["description"] != "top accounts" {
		t.Errorf("segment body = %v", body["segment"])
	}
}

func TestSegmentDelete_EmptyBodyReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "segment", "delete", "--id", "9")
	if exit != 0 {
		t.Fatalf("exit != 0")
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/segments/9" {
		t.Errorf("got %s %s, want DELETE /v1/segments/9", got.Method, got.Path)
	}
	receipt := decodeBody(t, []byte(stdout))
	if receipt["ok"] != true || receipt["deleted"] != "9" {
		t.Errorf("receipt = %v, want {ok:true, deleted:9}", receipt)
	}
}

func TestSendEmail_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"delivery_id":"d1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "send", "email",
		"--transactional-id", "3", "--to", "jane@example.com",
		"--identifier", "email=jane@example.com", "--message-data", `{"name":"Jane"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Path != "/v1/send/email" {
		t.Errorf("path = %q, want /v1/send/email", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["transactional_message_id"] != "3" || body["to"] != "jane@example.com" {
		t.Errorf("body = %v", body)
	}
	ids, ok := body["identifiers"].(map[string]any)
	if !ok || ids["email"] != "jane@example.com" {
		t.Errorf("identifiers = %v, want {email:jane@example.com}", body["identifiers"])
	}
	md, ok := body["message_data"].(map[string]any)
	if !ok || md["name"] != "Jane" {
		t.Errorf("message_data = %v", body["message_data"])
	}
}

func TestSendEmail_BadIdentifierIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "send", "email",
		"--transactional-id", "3", "--to", "j@e.co", "--identifier", "novalue")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for a malformed --identifier", exit)
	}
}

func TestExportDeliveries_RequiresExactlyOneSource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "export", "deliveries"); exit != 2 {
		t.Errorf("no-source exit = %d, want 2", exit)
	}
	if exit, _, _ := run(t, srv, "export", "deliveries", "--newsletter", "1", "--campaign", "2"); exit != 2 {
		t.Errorf("two-source exit = %d, want 2", exit)
	}
}

func TestMessageList_Filters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"messages":[]}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "message", "list", "--state", "bounced", "--type", "email", "--limit", "10"); exit != 0 {
		t.Fatalf("exit != 0")
	}
	q := parseQuery(t, got.Query)
	want := map[string]string{"state": "bounced", "type": "email", "limit": "10"}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query[%s] = %q, want %q", k, q.Get(k), v)
		}
	}
}

func TestParseIdentifiers(t *testing.T) {
	got, err := parseIdentifiers([]string{"email=a@b.co", "cio_id=xyz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"email": "a@b.co", "cio_id": "xyz"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("identifiers = %v, want %v", got, want)
	}
	if _, err := parseIdentifiers(nil); err == nil {
		t.Error("expected error for empty identifiers")
	}
}
