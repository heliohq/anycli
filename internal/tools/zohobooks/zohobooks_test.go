package zohobooks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const orgID = "460000000000369"

// --- credential + envelope contract ---------------------------------------

func TestMissingTokenExitsOneWithMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := runWithEnv(t, srv, map[string]string{}, "org", "list")
	if got.result.ExitCode != 1 {
		t.Fatalf("missing token exit = %d, want 1", got.result.ExitCode)
	}
	if !strings.Contains(got.stderr, EnvToken+" is not set") {
		t.Errorf("stderr = %q, want a %s-not-set message", got.stderr, EnvToken)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made without a token, got %d", len(reqs))
	}
}

func TestMissingTokenJSONError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := runWithEnv(t, srv, map[string]string{}, "org", "list", "--json")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(got.stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, got.stderr)
	}
	if env.Error.Kind != "credential" || !strings.Contains(env.Error.Message, "not set") {
		t.Errorf("error envelope = %+v, want kind=credential", env.Error)
	}
}

// --- auth header scheme ----------------------------------------------------

func TestAuthHeaderScheme(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/organizations": {status: 200, body: `{"code":0,"message":"success","organizations":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "org", "list")
	req := findReq(reqs, http.MethodGet, "/books/v3/organizations")
	if req == nil {
		t.Fatal("no request to /books/v3/organizations")
	}
	// Books requires the Zoho-oauthtoken scheme, NOT Bearer.
	if req.Auth != "Zoho-oauthtoken test-token" {
		t.Errorf("auth = %q, want Zoho-oauthtoken test-token", req.Auth)
	}
}

// --- organization_id propagation ------------------------------------------

func TestOrgListSendsNoOrganizationID(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/organizations": {status: 200, body: `{"code":0,"organizations":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "org", "list")
	if got.result.ExitCode != 0 {
		t.Fatalf("org list exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/books/v3/organizations")
	if req.Query.Has("organization_id") {
		t.Errorf("org list must NOT send organization_id, got %q", req.Query.Get("organization_id"))
	}
}

func TestOrgScopedCallPropagatesOrganizationID(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/invoices": {status: 200, body: `{"code":0,"invoices":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "invoice", "list", "--organization-id", orgID)
	if got.result.ExitCode != 0 {
		t.Fatalf("invoice list exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/books/v3/invoices")
	if got := req.Query.Get("organization_id"); got != orgID {
		t.Errorf("organization_id = %q, want %q", got, orgID)
	}
}

func TestOrgScopedCreatePropagatesOrganizationID(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /books/v3/contacts": {status: 201, body: `{"code":0,"contact":{"contact_id":"1"}}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "contact", "create", "--organization-id", orgID, "--data", `{"contact_name":"Acme"}`)
	req := findReq(reqs, http.MethodPost, "/books/v3/contacts")
	if req == nil {
		t.Fatal("no POST to /books/v3/contacts")
	}
	if got := req.Query.Get("organization_id"); got != orgID {
		t.Errorf("organization_id = %q, want %q", got, orgID)
	}
}

func TestMissingOrganizationIDIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "invoice", "list")
	if got.result.ExitCode != 2 {
		t.Fatalf("missing --organization-id exit = %d, want 2 (usage); stderr=%s", got.result.ExitCode, got.stderr)
	}
	if !strings.Contains(got.stderr, "org list") {
		t.Errorf("stderr should point the agent at `org list`: %s", got.stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call without an org id, got %d", len(reqs))
	}
}

// --- list filters + pagination --------------------------------------------

func TestInvoiceListPropagatesFiltersAndPagination(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/invoices": {status: 200, body: `{"code":0,"invoices":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "invoice", "list", "--organization-id", orgID,
		"--customer-id", "C1", "--status", "overdue", "--filter-by", "Status.Overdue",
		"--search-text", "acme", "--page", "2", "--per-page", "50")
	req := findReq(reqs, http.MethodGet, "/books/v3/invoices")
	q := req.Query
	for k, want := range map[string]string{
		"customer_id": "C1",
		"status":      "overdue",
		"filter_by":   "Status.Overdue",
		"search_text": "acme",
		"page":        "2",
		"per_page":    "50",
	} {
		if q.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), want)
		}
	}
}

func TestContactListPropagatesContactType(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/contacts": {status: 200, body: `{"code":0,"contacts":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "contact", "list", "--organization-id", orgID, "--contact-type", "vendor")
	req := findReq(reqs, http.MethodGet, "/books/v3/contacts")
	if got := req.Query.Get("contact_type"); got != "vendor" {
		t.Errorf("contact_type = %q, want vendor", got)
	}
}

func TestBillListPropagatesVendorID(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/bills": {status: 200, body: `{"code":0,"bills":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "bill", "list", "--organization-id", orgID, "--vendor-id", "V9")
	req := findReq(reqs, http.MethodGet, "/books/v3/bills")
	if got := req.Query.Get("vendor_id"); got != "V9" {
		t.Errorf("vendor_id = %q, want V9", got)
	}
}

func TestPaymentListHitsCustomerPayments(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/customerpayments": {status: 200, body: `{"code":0,"customerpayments":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "payment", "list", "--organization-id", orgID, "--customer-id", "C7")
	if got.result.ExitCode != 0 {
		t.Fatalf("payment list exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/books/v3/customerpayments")
	if req == nil {
		t.Fatal("payment list must hit /books/v3/customerpayments")
	}
	if got := req.Query.Get("customer_id"); got != "C7" {
		t.Errorf("customer_id = %q, want C7", got)
	}
}

// --- get by id -------------------------------------------------------------

func TestInvoiceGetPathAndOrgID(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /books/v3/invoices/555": {status: 200, body: `{"code":0,"invoice":{"invoice_id":"555"}}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "invoice", "get", "--organization-id", orgID, "--id", "555")
	if got.result.ExitCode != 0 {
		t.Fatalf("invoice get exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/books/v3/invoices/555")
	if req == nil {
		t.Fatal("no GET to /books/v3/invoices/555")
	}
	if got := req.Query.Get("organization_id"); got != orgID {
		t.Errorf("organization_id = %q, want %q", got, orgID)
	}
}

func TestGetRequiresID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "invoice", "get", "--organization-id", orgID)
	if got.result.ExitCode != 2 {
		t.Fatalf("missing --id exit = %d, want 2", got.result.ExitCode)
	}
}

// --- create raw-body passthrough ------------------------------------------

func TestCreateSendsRawBodyVerbatim(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /books/v3/invoices": {status: 201, body: `{"code":0,"invoice":{"invoice_id":"9"}}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	payload := `{"customer_id":"C1","line_items":[{"item_id":"I1","rate":10}]}`
	got := run(t, srv, "invoice", "create", "--organization-id", orgID, "--data", payload)
	if got.result.ExitCode != 0 {
		t.Fatalf("invoice create exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPost, "/books/v3/invoices")
	// Books create bodies are flat JSON objects, NOT a {"data":[…]} wrapper.
	body := bodyMap(t, req.Body)
	if body["customer_id"] != "C1" {
		t.Errorf("body customer_id = %v, want C1 (raw passthrough, no wrapper)", body["customer_id"])
	}
	if _, wrapped := body["data"]; wrapped {
		t.Error("create body must NOT wrap into {\"data\":…} — Books takes a flat body")
	}
	if req.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", req.ContentType)
	}
}

func TestCreateRequiresData(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "invoice", "create", "--organization-id", orgID)
	if got.result.ExitCode != 2 {
		t.Fatalf("missing --data exit = %d, want 2", got.result.ExitCode)
	}
}

func TestCreateRejectsInvalidJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "contact", "create", "--organization-id", orgID, "--data", `{not json`)
	if got.result.ExitCode != 2 {
		t.Fatalf("invalid --data exit = %d, want 2", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call for invalid JSON, got %d", len(reqs))
	}
}

// --- error surfacing (integer code) ---------------------------------------

func TestAPIErrorPlainAndExitOne(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /books/v3/invoices": {status: 400, body: `{"code":57,"message":"You are not authorized to perform this operation"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "invoice", "list", "--organization-id", orgID)
	if got.result.ExitCode != 1 {
		t.Fatalf("API error exit = %d, want 1", got.result.ExitCode)
	}
	// A 400 permission error must NOT invalidate a valid token.
	if got.result.CredentialRejected {
		t.Error("a 400 permission error must NOT reject the credential")
	}
	if !strings.Contains(got.stderr, "57") || !strings.Contains(got.stderr, "HTTP 400") {
		t.Errorf("stderr should carry Books code + status: %s", got.stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /books/v3/items": {status: 400, body: `{"code":57,"message":"not authorized"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "item", "list", "--organization-id", orgID, "--json")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(got.stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, got.stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind=api status=400", env.Error)
	}
}

func TestInvalidTokenRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	// Official Books mapping: HTTP 401 = "Unauthorized (Invalid AuthToken)".
	routes := map[string]stub{
		"GET /books/v3/organizations": {status: 401, body: `{"code":57,"message":"Invalid AuthToken"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "org", "list")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	if !got.result.CredentialRejected {
		t.Error("HTTP 401 (Invalid AuthToken) must reject the credential so the engine re-auths")
	}
}

// TestSuccessEnvelopeWithNonZeroCodeIsError is the defensive path: a 2xx whose
// body `code` is non-zero is still an error (Books' integer code is the
// authoritative success signal), surfaced as exit 1 without rejecting the token.
func TestSuccessEnvelopeWithNonZeroCodeIsError(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /books/v3/invoices": {status: 200, body: `{"code":15,"message":"Invalid value passed for filter_by."}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "invoice", "list", "--organization-id", orgID, "--filter-by", "Bogus")
	if got.result.ExitCode != 1 {
		t.Fatalf("2xx-with-nonzero-code exit = %d, want 1; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if got.result.CredentialRejected {
		t.Error("a 2xx body-code error must NOT reject the credential")
	}
	if !strings.Contains(got.stderr, "15") {
		t.Errorf("stderr should carry the body code 15: %s", got.stderr)
	}
}

// TestSuccessEnvelopePrintedVerbatim asserts the provider envelope (code 0 +
// page_context) reaches stdout unchanged so the agent can page.
func TestSuccessEnvelopePrintedVerbatim(t *testing.T) {
	var reqs []capturedRequest
	body := `{"code":0,"message":"success","invoices":[{"invoice_id":"1"}],"page_context":{"has_more_page":true}}`
	routes := map[string]stub{"GET /books/v3/invoices": {status: 200, body: body}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "invoice", "list", "--organization-id", orgID)
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", got.result.ExitCode)
	}
	if !strings.Contains(got.stdout, `"has_more_page":true`) {
		t.Errorf("stdout should carry the verbatim envelope incl. page_context: %s", got.stdout)
	}
}

// --- cobra usage semantics -------------------------------------------------

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "invoice", "frobnicate")
	if got.result.ExitCode != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call for an unknown subcommand, got %d", len(reqs))
	}
}

func TestBareGroupShowsHelpExitZero(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "invoice")
	if got.result.ExitCode != 0 {
		t.Fatalf("bare group exit = %d, want 0 (help)", got.result.ExitCode)
	}
}
