package lusha

import (
	"net/http"
	"testing"
)

func TestContactEnrichEmailBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"requestId":"req-1","results":[{"id":"1","emails":[{"email":"a@b.com"}]}],"billing":{"creditsCharged":2,"resultsReturned":1}}`, &got)
	defer srv.Close()

	code, out, _ := run(t, srv, "contact", "enrich", "--email", "orit@lusha.com", "--reveal", "emails,phones")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/contacts/search-and-enrich" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	contacts, ok := body["contacts"].([]any)
	if !ok || len(contacts) != 1 {
		t.Fatalf("contacts = %v", body["contacts"])
	}
	item := contacts[0].(map[string]any)
	if item["email"] != "orit@lusha.com" {
		t.Errorf("contact item = %v", item)
	}
	reveal, ok := body["reveal"].([]any)
	if !ok || len(reveal) != 2 || reveal[0] != "emails" || reveal[1] != "phones" {
		t.Errorf("reveal = %v", body["reveal"])
	}
	// Envelope: data is an array, meta carries credits + request_id.
	env := decodeStdout(t, out)
	if _, ok := env["data"].([]any); !ok {
		t.Errorf("data is not an array: %v", env["data"])
	}
	meta := env["meta"].(map[string]any)
	if meta["credits_charged"].(float64) != 2 || meta["request_id"] != "req-1" {
		t.Errorf("meta = %v", meta)
	}
}

func TestContactEnrichNamePairBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "enrich", "--first-name", "Orit", "--last-name", "Shilvock", "--company-domain", "lusha.com")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	item := decodeBody(t, got.Body)["contacts"].([]any)[0].(map[string]any)
	if item["firstName"] != "Orit" || item["lastName"] != "Shilvock" || item["companyDomain"] != "lusha.com" {
		t.Errorf("item = %v", item)
	}
	if _, ok := item["reveal"]; ok {
		t.Errorf("reveal should be absent on the item")
	}
}

func TestContactEnrichNoIdentifierIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "contact", "enrich")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API")
	}
}

func TestContactRevealBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"requestId":"r2","results":[{"id":"1"}],"billing":{"creditsCharged":1,"resultsReturned":1}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "reveal", "--id", "4389064654", "--id", "4389064624", "--reveal", "emails")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/contacts/enrich" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	ids := body["ids"].([]any)
	if len(ids) != 2 || ids[0] != "4389064654" {
		t.Errorf("ids = %v", ids)
	}
	if reveal := body["reveal"].([]any); len(reveal) != 1 || reveal[0] != "emails" {
		t.Errorf("reveal = %v", body["reveal"])
	}
}

func TestContactRevealInvalidRevealValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "contact", "reveal", "--id", "1", "--reveal", "competitors")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API on invalid reveal")
	}
	if stderr == "" {
		t.Errorf("expected an error message")
	}
}

func TestContactSearchBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"requestId":"r3","results":[{"id":"1"}],"pagination":{"page":0,"size":25,"total":150},"billing":{"creditsCharged":25}}`, &got)
	defer srv.Close()

	code, out, _ := run(t, srv, "contact", "search",
		"--filters", `{"contacts":{"include":{"jobTitles":["VP Sales"]}}}`,
		"--size", "25", "--page", "0")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/contacts/prospecting" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	pg := body["pagination"].(map[string]any)
	if pg["page"].(float64) != 0 || pg["size"].(float64) != 25 {
		t.Errorf("pagination = %v", pg)
	}
	filters := body["filters"].(map[string]any)
	if _, ok := filters["contacts"]; !ok {
		t.Errorf("filters = %v", filters)
	}
	if _, ok := body["options"]; ok {
		t.Errorf("options should be absent when --include-partial not set")
	}
	// Search envelope: has_more derived from pagination.
	meta := decodeStdout(t, out)["meta"].(map[string]any)
	if meta["has_more"] != true || meta["total"].(float64) != 150 {
		t.Errorf("meta = %v", meta)
	}
}

func TestContactSearchMissingFiltersIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "contact", "search")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API")
	}
}

func TestContactSearchIncludePartial(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "search", "--filters", `{"contacts":{"include":{}}}`, "--include-partial")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	opts := decodeBody(t, got.Body)["options"].(map[string]any)
	if opts["includePartialProfiles"] != true {
		t.Errorf("options = %v", opts)
	}
}
