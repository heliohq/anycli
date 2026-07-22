package xero

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// connBody builds a /connections response array from name→id pairs.
func connBody(pairs ...[2]string) string {
	var items []map[string]any
	for _, p := range pairs {
		items = append(items, map[string]any{
			"id":         "conn-" + p[1],
			"tenantId":   p[1],
			"tenantName": p[0],
			"tenantType": "ORGANISATION",
		})
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func TestConnectionsListsOrgsVerbatimNoTenantHeader(t *testing.T) {
	var reqs []capturedRequest
	body := connBody([2]string{"Demo Co", tenantA})
	srv := newMux(t, &reqs, map[string]stub{"GET /connections": {200, body}})
	defer srv.Close()

	out, errb, res := run(t, srv, "connections")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/connections")
	if req == nil {
		t.Fatal("no GET /connections")
	}
	if req.Auth != "Bearer tok-test" {
		t.Errorf("Authorization = %q", req.Auth)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q", req.Accept)
	}
	if req.Tenant != "" {
		t.Errorf("connections must not send Xero-Tenant-Id, got %q", req.Tenant)
	}
	if strings.TrimSpace(out) != strings.TrimSpace(body) {
		t.Errorf("stdout not verbatim connections body:\n%s", out)
	}
}

func TestSingleOrgAutoResolvesTenantHeader(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /connections":              {200, connBody([2]string{"Demo Co", tenantA})},
		"GET /api.xro/2.0/Organisation": {200, `{"Organisations":[{"Name":"Demo Co"}]}`},
	})
	defer srv.Close()

	out, errb, res := run(t, srv, "organisation", "get")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Organisation")
	if req == nil {
		t.Fatal("no GET /api.xro/2.0/Organisation")
	}
	if req.Tenant != tenantA {
		t.Errorf("Xero-Tenant-Id = %q, want %q", req.Tenant, tenantA)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q", req.Accept)
	}
	if !strings.Contains(out, "Demo Co") {
		t.Errorf("stdout missing org body: %s", out)
	}
}

func TestMultiOrgWithoutTenantExits2WithCandidates(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /connections": {200, connBody([2]string{"Alpha Ltd", tenantA}, [2]string{"Beta Ltd", tenantB})},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "invoice", "list")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", res.ExitCode, errb)
	}
	if findReq(reqs, "GET", "/api.xro/2.0/Invoices") != nil {
		t.Error("must not call Invoices when tenant is ambiguous")
	}
	for _, want := range []string{"Alpha Ltd", tenantA, "Beta Ltd", tenantB, "--tenant"} {
		if !strings.Contains(errb, want) {
			t.Errorf("stderr missing %q:\n%s", want, errb)
		}
	}
}

func TestZeroOrgExits1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /connections": {200, `[]`}})
	defer srv.Close()

	_, errb, res := run(t, srv, "contact", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", res.ExitCode, errb)
	}
	if !strings.Contains(strings.ToLower(errb), "no xero organisation") {
		t.Errorf("stderr = %s", errb)
	}
}

func TestTenantGUIDFlagSkipsConnections(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Contacts": {200, `{"Contacts":[]}`},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "contact", "list", "--tenant", tenantB)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	if findReq(reqs, "GET", "/connections") != nil {
		t.Error("a GUID --tenant must not call /connections")
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Contacts")
	if req == nil || req.Tenant != tenantB {
		t.Fatalf("Contacts tenant = %v", req)
	}
}

func TestTenantNameFlagResolvesViaConnections(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /connections":          {200, connBody([2]string{"Alpha Ltd", tenantA}, [2]string{"Beta Ltd", tenantB})},
		"GET /api.xro/2.0/Contacts": {200, `{"Contacts":[]}`},
	})
	defer srv.Close()

	// Case-insensitive name match picks the right tenantId.
	_, errb, res := run(t, srv, "contact", "list", "--tenant", "beta ltd")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Contacts")
	if req == nil || req.Tenant != tenantB {
		t.Fatalf("resolved tenant wrong: %v", req)
	}
}

func TestUnknownTenantNameExits2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /connections": {200, connBody([2]string{"Alpha Ltd", tenantA})},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "contact", "list", "--tenant", "Nonexistent")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", res.ExitCode, errb)
	}
	if !strings.Contains(errb, "Nonexistent") {
		t.Errorf("stderr = %s", errb)
	}
}

func TestListForwardsQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Invoices": {200, `{"Invoices":[]}`},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "invoice", "list", "--tenant", tenantA,
		"--query", "where=Status==\"AUTHORISED\"", "--query", "page=2")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Invoices")
	if req.Query.Get("where") != `Status=="AUTHORISED"` {
		t.Errorf("where = %q", req.Query.Get("where"))
	}
	if req.Query.Get("page") != "2" {
		t.Errorf("page = %q", req.Query.Get("page"))
	}
}

func TestGetByID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Invoices/INV-001": {200, `{"Invoices":[{"InvoiceID":"x"}]}`},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "invoice", "get", "INV-001", "--tenant", tenantA)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	if findReq(reqs, "GET", "/api.xro/2.0/Invoices/INV-001") == nil {
		t.Fatal("no GET Invoices/INV-001")
	}
}

func TestCreateUsesPUTWithBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /api.xro/2.0/Invoices": {200, `{"Invoices":[{"InvoiceID":"new"}]}`},
	})
	defer srv.Close()

	payload := `{"Invoices":[{"Type":"ACCREC","Contact":{"ContactID":"c1"}}]}`
	_, errb, res := run(t, srv, "invoice", "create", "--tenant", tenantA, "--data", payload)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "PUT", "/api.xro/2.0/Invoices")
	if req == nil {
		t.Fatal("no PUT Invoices")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q", req.ContentType)
	}
	got := bodyMap(t, req.Body)
	if _, ok := got["Invoices"]; !ok {
		t.Errorf("body not forwarded verbatim: %s", req.Body)
	}
}

func TestUpdateUsesPOST(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /api.xro/2.0/Contacts": {200, `{"Contacts":[]}`},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "contact", "update", "--tenant", tenantA, "--data", `{"Contacts":[{"ContactID":"c1","Name":"n"}]}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	if findReq(reqs, "POST", "/api.xro/2.0/Contacts") == nil {
		t.Fatal("no POST Contacts")
	}
}

func TestInvoiceEmail(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /api.xro/2.0/Invoices/INV-9/Email": {204, ``},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "invoice", "email", "INV-9", "--tenant", tenantA)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	if findReq(reqs, "POST", "/api.xro/2.0/Invoices/INV-9/Email") == nil {
		t.Fatal("no POST Invoices/INV-9/Email")
	}
}

func TestReportMapsName(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Reports/ProfitAndLoss": {200, `{"Reports":[]}`},
	})
	defer srv.Close()

	_, errb, res := run(t, srv, "report", "pnl", "--tenant", tenantA, "--query", "periods=3")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Reports/ProfitAndLoss")
	if req == nil {
		t.Fatal("no ProfitAndLoss report call")
	}
	if req.Query.Get("periods") != "3" {
		t.Errorf("periods = %q", req.Query.Get("periods"))
	}
}

func TestFetchRawPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/CreditNotes": {200, `{"CreditNotes":[]}`},
	})
	defer srv.Close()

	// Leading slash should be tolerated.
	_, errb, res := run(t, srv, "fetch", "/CreditNotes", "--tenant", tenantA, "--query", "where=Status==\"PAID\"")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/CreditNotes")
	if req == nil {
		t.Fatal("no fetch CreditNotes call")
	}
	if req.Query.Get("where") == "" {
		t.Errorf("fetch dropped query: %v", req.Query)
	}
}

func TestAPIErrorExit1JSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Invoices": {400, `{"Type":"ValidationException","Message":"bad","Elements":[]}`},
	})
	defer srv.Close()

	out, errb, res := run(t, srv, "invoice", "list", "--tenant", tenantA, "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if out != "" {
		t.Errorf("stdout should be empty on error, got %q", out)
	}
	var env struct {
		Error struct {
			Tool    string          `json:"tool"`
			Code    string          `json:"code"`
			Status  int             `json:"status"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, errb)
	}
	if env.Error.Tool != "xero" || env.Error.Code != "api_error" || env.Error.Status != 400 {
		t.Errorf("envelope = %+v", env.Error)
	}
	if !strings.Contains(string(env.Error.Details), "ValidationException") {
		t.Errorf("details did not surface Xero body: %s", env.Error.Details)
	}
}

func TestMissingTokenExit1(t *testing.T) {
	_, errb, res := runWithEnv(t, nil, map[string]string{}, "connections")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errb, "XERO_ACCESS_TOKEN") {
		t.Errorf("stderr = %s", errb)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	_, _, res := run(t, nil, "contact", "bogus")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
}

func TestDefaultTenantFromEnv(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api.xro/2.0/Accounts": {200, `{"Accounts":[]}`},
	})
	defer srv.Close()

	_, errb, res := runWithEnv(t, srv, map[string]string{"access_token": "tok-test", "tenant": tenantA}, "account", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errb)
	}
	req := findReq(reqs, "GET", "/api.xro/2.0/Accounts")
	if req == nil || req.Tenant != tenantA {
		t.Fatalf("env tenant not used: %v", req)
	}
	if findReq(reqs, "GET", "/connections") != nil {
		t.Error("env tenant GUID must not call /connections")
	}
}
