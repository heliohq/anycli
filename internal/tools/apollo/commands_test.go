package apollo

import (
	"net/http"
	"testing"
)

func TestPeopleEnrichPostsMatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"person":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "people", "enrich", "--email", "jane@acme.com", "--org-domain", "acme.com", "--reveal-phone")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/people/match" {
		t.Fatalf("request = %s %s, want POST /people/match", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["email"] != "jane@acme.com" || b["domain"] != "acme.com" {
		t.Fatalf("body = %v, want email+domain set", b)
	}
	if b["reveal_phone_number"] != true {
		t.Fatalf("reveal_phone_number = %v, want true", b["reveal_phone_number"])
	}
}

func TestPeopleSearchPathAndPaginationInBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"people":[],"pagination":{"page":2}}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "people", "search", "--title", "cto", "--seniority", "c_suite", "--page", "2", "--per-page", "25")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/mixed_people/api_search" {
		t.Fatalf("request = %s %s, want POST /mixed_people/api_search", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["page"] != float64(2) || b["per_page"] != float64(25) {
		t.Fatalf("pagination = page:%v per_page:%v, want 2/25", b["page"], b["per_page"])
	}
	titles, ok := b["person_titles"].([]any)
	if !ok || len(titles) != 1 || titles[0] != "cto" {
		t.Fatalf("person_titles = %v, want [cto]", b["person_titles"])
	}
	// pagination passthrough: the provider's own page field survives verbatim.
	if want := `"pagination":{"page":2}`; !contains(stdout, want) {
		t.Fatalf("stdout = %q, want provider pagination passthrough", stdout)
	}
}

func TestOrgSearchPathAndPaginationInBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"organizations":[],"pagination":{"page":2}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "org", "search", "--industry", "software", "--location", "California", "--page", "2", "--per-page", "25")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/mixed_companies/search" {
		t.Fatalf("request = %s %s, want POST /mixed_companies/search", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["page"] != float64(2) || b["per_page"] != float64(25) {
		t.Fatalf("pagination = page:%v per_page:%v, want 2/25", b["page"], b["per_page"])
	}
	tags, ok := b["q_organization_keyword_tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "software" {
		t.Fatalf("q_organization_keyword_tags = %v, want [software]", b["q_organization_keyword_tags"])
	}
	locs, ok := b["organization_locations"].([]any)
	if !ok || len(locs) != 1 || locs[0] != "California" {
		t.Fatalf("organization_locations = %v, want [California]", b["organization_locations"])
	}
}

func TestOrgEnrichUsesGetQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"organization":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "org", "enrich", "--domain", "acme.com")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodGet || got.Path != "/organizations/enrich" {
		t.Fatalf("request = %s %s, want GET /organizations/enrich", got.Method, got.Path)
	}
	if got.Query != "domain=acme.com" {
		t.Fatalf("query = %q, want domain=acme.com", got.Query)
	}
}

func TestContactsUpdateUsesPatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"contact":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contacts", "update", "c123", "--stage-id", "st1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPatch || got.Path != "/contacts/c123" {
		t.Fatalf("request = %s %s, want PATCH /contacts/c123", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["contact_stage_id"] != "st1" {
		t.Fatalf("contact_stage_id = %v, want st1", b["contact_stage_id"])
	}
}

func TestContactsStagesUsesGetUnderscorePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contacts", "stages")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodGet || got.Path != "/contact_stages" {
		t.Fatalf("request = %s %s, want GET /contact_stages", got.Method, got.Path)
	}
}

func TestAccountsUpdateUsesPatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"account":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "accounts", "update", "a1", "--name", "Acme")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPatch || got.Path != "/accounts/a1" {
		t.Fatalf("request = %s %s, want PATCH /accounts/a1", got.Method, got.Path)
	}
}

func TestSequencesListUsesPostSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"emailer_campaigns":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sequences", "list", "--q", "outbound")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/emailer_campaigns/search" {
		t.Fatalf("request = %s %s, want POST /emailer_campaigns/search", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["q_name"] != "outbound" {
		t.Fatalf("q_name = %v, want outbound", b["q_name"])
	}
}

func TestSequencesAddPathAndBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sequences", "add", "seq9",
		"--contact-ids", "c1", "--contact-ids", "c2", "--email-account-id", "ea1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/emailer_campaigns/seq9/add_contact_ids" {
		t.Fatalf("request = %s %s, want POST /emailer_campaigns/seq9/add_contact_ids", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	ids, ok := b["contact_ids"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("contact_ids = %v, want [c1 c2]", b["contact_ids"])
	}
	if b["send_email_from_email_account_id"] != "ea1" {
		t.Fatalf("send mailbox = %v, want ea1", b["send_email_from_email_account_id"])
	}
}

func TestSequencesAddRequiresContactIDs(t *testing.T) {
	// Missing the required --contact-ids flag is a usage error (exit 2).
	exit, _, _ := run(t, nil, "sequences", "add", "seq9")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for missing required flag", exit)
	}
}

func TestTasksCreatePostsBulk(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "tasks", "create", "--contact-id", "c1", "--type", "call", "--priority", "high")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/tasks/bulk_create" {
		t.Fatalf("request = %s %s, want POST /tasks/bulk_create", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["type"] != "call" || b["priority"] != "high" {
		t.Fatalf("body = %v, want type/priority set", b)
	}
}

func TestDealsSearchUsesGetQueryPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"opportunities":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "deals", "search", "--page", "3")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodGet || got.Path != "/opportunities/search" {
		t.Fatalf("request = %s %s, want GET /opportunities/search", got.Method, got.Path)
	}
	if got.Query != "page=3" {
		t.Fatalf("query = %q, want page=3", got.Query)
	}
}

func TestDealsUpdateUsesPatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "deals", "update", "o1", "--stage-id", "s2", "--amount", "5000")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPatch || got.Path != "/opportunities/o1" {
		t.Fatalf("request = %s %s, want PATCH /opportunities/o1", got.Method, got.Path)
	}
}

func TestEmailAccountsListPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "email-accounts", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodGet || got.Path != "/email_accounts" {
		t.Fatalf("request = %s %s, want GET /email_accounts", got.Method, got.Path)
	}
}

func TestBodyPassthroughMergedWithTypedFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// --body supplies an arbitrary Apollo filter the typed flags do not name;
	// typed flags augment the same object.
	exit, _, _ := run(t, srv, "people", "search",
		"--body", `{"person_locations":["California"]}`, "--title", "vp")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	b := decodeBody(t, got.Body)
	locs, ok := b["person_locations"].([]any)
	if !ok || len(locs) != 1 || locs[0] != "California" {
		t.Fatalf("person_locations = %v, want [California] from --body", b["person_locations"])
	}
	titles, ok := b["person_titles"].([]any)
	if !ok || titles[0] != "vp" {
		t.Fatalf("person_titles = %v, want [vp] from typed flag", b["person_titles"])
	}
}

func TestInvalidBodyJSONIsUsageError(t *testing.T) {
	exit, _, stderr := run(t, nil, "contacts", "search", "--body", "{not json")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for invalid --body JSON", exit)
	}
	if !contains(stderr, "not a valid JSON object") {
		t.Fatalf("stderr = %q, want invalid-JSON message", stderr)
	}
}

// contains is a tiny substring helper to keep assertions readable.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
