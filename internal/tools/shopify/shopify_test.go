package shopify

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeStore(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"myshop", "myshop.myshopify.com"},
		{"myshop.myshopify.com", "myshop.myshopify.com"},
		{"https://myshop.myshopify.com", "myshop.myshopify.com"},
		{"https://myshop.myshopify.com/admin", "myshop.myshopify.com"},
		{"  myshop  ", "myshop.myshopify.com"},
	}
	for _, tc := range cases {
		if got := normalizeStore(tc.in); got != tc.want {
			t.Errorf("normalizeStore(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEndpointConstruction(t *testing.T) {
	// Default version, bare shop name.
	c := &client{svc: &Service{}, store: "myshop"}
	got, err := c.endpoint("")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if want := "https://myshop.myshopify.com/admin/api/2026-07/graphql.json"; got != want {
		t.Errorf("endpoint = %q, want %q", got, want)
	}

	// --api-version flag overrides the pinned default.
	if got, _ := c.endpoint("2025-10"); !strings.Contains(got, "/admin/api/2025-10/graphql.json") {
		t.Errorf("api-version override not honored: %q", got)
	}

	// Service.APIVersion is the fallback when no flag is passed.
	c2 := &client{svc: &Service{APIVersion: "2026-01"}, store: "acme.myshopify.com"}
	if got, _ := c2.endpoint(""); got != "https://acme.myshopify.com/admin/api/2026-01/graphql.json" {
		t.Errorf("service api-version fallback wrong: %q", got)
	}

	// BaseURL override wins outright (test seam).
	c3 := &client{svc: &Service{BaseURL: "http://127.0.0.1:9/x"}, store: "ignored"}
	if got, _ := c3.endpoint(""); got != "http://127.0.0.1:9/x" {
		t.Errorf("BaseURL override not honored: %q", got)
	}
}

func TestShopInfoRequestAndOutput(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{"data":{"shop":{"name":"My Store","currencyCode":"USD"}}}`)
	defer srv.Close()

	res := runAgainst(t, srv, "shop", "info")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Method != "POST" {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.Auth != "shpat_test" {
		t.Errorf("X-Shopify-Access-Token = %q, want shpat_test", r.Auth)
	}
	if r.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", r.ContentType)
	}
	q, _ := bodyOf(t, r.Body)
	if !strings.Contains(q, "shop") {
		t.Errorf("query does not reference shop: %s", q)
	}
	out := decodeJSON(t, res.stdout)
	shop, _ := out["shop"].(map[string]any)
	if shop == nil || shop["name"] != "My Store" {
		t.Errorf("unexpected stdout shop payload: %s", res.stdout)
	}
}

func TestProductListUnwrapsConnection(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"products":{"edges":[
		{"node":{"id":"gid://shopify/Product/1","title":"Tee","status":"ACTIVE"}},
		{"node":{"id":"gid://shopify/Product/2","title":"Hat","status":"DRAFT"}}
	],"pageInfo":{"hasNextPage":true,"endCursor":"CUR2"}}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "list", "--limit", "5", "--after", "CUR1", "--query", "status:active")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	_, vars := bodyOf(t, reqs[0].Body)
	if vars["first"] != float64(5) {
		t.Errorf("first = %v, want 5", vars["first"])
	}
	if vars["after"] != "CUR1" {
		t.Errorf("after = %v, want CUR1", vars["after"])
	}
	if vars["query"] != "status:active" {
		t.Errorf("query = %v, want status:active", vars["query"])
	}
	out := decodeJSON(t, res.stdout)
	products, ok := out["products"].([]any)
	if !ok || len(products) != 2 {
		t.Fatalf("products not unwrapped to a 2-element list: %s", res.stdout)
	}
	pi, _ := out["page_info"].(map[string]any)
	if pi == nil || pi["has_next_page"] != true || pi["end_cursor"] != "CUR2" {
		t.Errorf("page_info not flattened: %s", res.stdout)
	}
}

func TestProductGetNormalizesGID(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{"data":{"product":{"id":"gid://shopify/Product/42","title":"Tee"}}}`)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "get", "42")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	_, vars := bodyOf(t, reqs[0].Body)
	if vars["id"] != "gid://shopify/Product/42" {
		t.Errorf("id = %v, want gid://shopify/Product/42", vars["id"])
	}
}

func TestProductUpdateUserErrorsFail(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"productUpdate":{"product":null,"userErrors":[{"field":["status"],"message":"is invalid"}]}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "update", "42", "--status", "ACTIVE")
	if res.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (userErrors are failures); stderr = %s", res.result.ExitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "status: is invalid") {
		t.Errorf("stderr missing userErrors detail: %s", res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("no stdout expected on a failed mutation, got: %s", res.stdout)
	}
}

func TestProductUpdateSuccess(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"productUpdate":{"product":{"id":"gid://shopify/Product/42","status":"ACTIVE"},"userErrors":[]}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "update", "42", "--status", "ACTIVE")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	out := decodeJSON(t, res.stdout)
	if out["status"] != "ACTIVE" {
		t.Errorf("unexpected product payload: %s", res.stdout)
	}
}

func TestProductUpdateBadStatusIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "update", "42", "--status", "NOPE")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", res.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("a parse-time usage error must not hit the API")
	}
}

func TestProductUpdateRequiresAField(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "update", "42")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (no field flags)", res.result.ExitCode)
	}
}

func TestInventoryAdjustBuildsChanges(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"inventoryAdjustQuantities":{"inventoryAdjustmentGroup":{"reason":"correction","changes":[{"name":"available","delta":5}]},"userErrors":[]}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "inventory", "adjust", "--item", "11", "--location", "22", "--delta", "5")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	query, vars := bodyOf(t, reqs[0].Body)
	// As of Admin API 2026-04 the @idempotent directive is mandatory on
	// inventoryAdjustQuantities; the tool pins 2026-07, so the mutation must
	// carry it, bound to the $idempotencyKey variable, and the key must default
	// to a non-empty per-invocation UUID.
	if !strings.Contains(query, "@idempotent(key: $idempotencyKey)") {
		t.Errorf("mutation missing required @idempotent directive: %s", query)
	}
	if key, _ := vars["idempotencyKey"].(string); key == "" {
		t.Errorf("idempotencyKey variable must default to a non-empty UUID, got %v", vars["idempotencyKey"])
	}
	input, _ := vars["input"].(map[string]any)
	changes, _ := input["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %v", input["changes"])
	}
	ch, _ := changes[0].(map[string]any)
	if ch["inventoryItemId"] != "gid://shopify/InventoryItem/11" {
		t.Errorf("inventoryItemId = %v", ch["inventoryItemId"])
	}
	if ch["locationId"] != "gid://shopify/Location/22" {
		t.Errorf("locationId = %v", ch["locationId"])
	}
	if ch["delta"] != float64(5) {
		t.Errorf("delta = %v, want 5", ch["delta"])
	}
}

// TestInventoryAdjustHonorsIdempotencyKeyFlag verifies an explicit
// --idempotency-key is forwarded verbatim (deliberate cross-invocation retry
// safety) instead of a fresh UUID.
func TestInventoryAdjustHonorsIdempotencyKeyFlag(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"inventoryAdjustQuantities":{"inventoryAdjustmentGroup":{"reason":"correction","changes":[]},"userErrors":[]}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "inventory", "adjust", "--item", "11", "--location", "22", "--delta", "5", "--idempotency-key", "fixed-key-123")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	_, vars := bodyOf(t, reqs[0].Body)
	if key, _ := vars["idempotencyKey"].(string); key != "fixed-key-123" {
		t.Errorf("idempotencyKey = %v, want fixed-key-123", vars["idempotencyKey"])
	}
}

func TestInventoryAdjustRequiresDelta(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "inventory", "adjust", "--item", "11", "--location", "22")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (missing delta)", res.result.ExitCode)
	}
}

func TestUnauthorizedIsCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 401, `{"errors":"[API] Invalid API key or access token"}`)
	defer srv.Close()

	res := runAgainst(t, srv, "shop", "info")
	if res.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.result.ExitCode)
	}
	if !res.result.CredentialRejected {
		t.Errorf("401 should classify as a credential rejection so the token gateway refreshes")
	}
}

func TestGraphQLTopLevelErrorsFail(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{"errors":[{"message":"Field 'nope' doesn't exist"}]}`)
	defer srv.Close()

	res := runAgainst(t, srv, "shop", "info")
	if res.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 for a GraphQL top-level error", res.result.ExitCode)
	}
	if !strings.Contains(res.stderr, "doesn't exist") {
		t.Errorf("stderr missing GraphQL error: %s", res.stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 401, `unauthorized`)
	defer srv.Close()

	res := runAgainst(t, srv, "--json", "shop", "info")
	if res.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.result.ExitCode)
	}
	env := decodeJSON(t, res.stderr)
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["kind"] != "api" {
		t.Fatalf("expected api error envelope, got: %s", res.stderr)
	}
	if errObj["status"] != float64(401) {
		t.Errorf("status = %v, want 401", errObj["status"])
	}
}

func TestGraphQLPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{"data":{"shop":{"name":"X"}}}`)
	defer srv.Close()

	res := runAgainst(t, srv, "graphql", "--query", "query{ shop { name } }", "--variables", `{"k":"v"}`)
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	q, vars := bodyOf(t, reqs[0].Body)
	if !strings.Contains(q, "shop { name }") {
		t.Errorf("passthrough query not forwarded: %s", q)
	}
	if vars["k"] != "v" {
		t.Errorf("passthrough variables not forwarded: %v", vars)
	}
}

func TestGraphQLPassthroughBadVariablesIsUsage(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "graphql", "--query", "{shop{name}}", "--variables", "{not json")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (bad JSON variables)", res.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("invalid --variables must fail before any request")
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	svc := &Service{}
	var out, errb strings.Builder
	svc.Out = &out
	svc.Err = &errb
	res, err := svc.Execute(context.Background(), []string{"shop", "info"}, map[string]string{EnvStore: "s.myshopify.com"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errb.String(), EnvAccessToken) {
		t.Errorf("stderr should name the missing token env: %s", errb.String())
	}
}

func TestMissingStoreFailsFast(t *testing.T) {
	svc := &Service{}
	var out, errb strings.Builder
	svc.Out = &out
	svc.Err = &errb
	res, err := svc.Execute(context.Background(), []string{"shop", "info"}, map[string]string{EnvAccessToken: "shpat_x"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errb.String(), EnvStore) {
		t.Errorf("stderr should name the missing store env: %s", errb.String())
	}
}

// TestMissingCredentialJSONKindMatchesExit pins the emitted error kind to the
// exit code for both missing-credential cases. Absent credentials are a
// runtime/environment failure (the connection was never injected), so they are
// exit 1 — and under --json the kind must agree ("api" = the API/runtime
// category), never "usage" (which would imply exit 2 and a caller-fixable flag
// mistake). Regression guard for the kind/exit disagreement fixed alongside the
// same quickbooks fix.
func TestMissingCredentialJSONKindMatchesExit(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"missing token", map[string]string{EnvStore: "myshop.myshopify.com"}},
		{"missing store", map[string]string{EnvAccessToken: "shpat_x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{}
			var out, errb strings.Builder
			svc.Out = &out
			svc.Err = &errb
			res, err := svc.Execute(context.Background(), []string{"--json", "shop", "info"}, tc.env)
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if res.ExitCode != 1 {
				t.Fatalf("exit = %d, want 1", res.ExitCode)
			}
			env := decodeJSON(t, strings.TrimSpace(errb.String()))
			errObj, _ := env["error"].(map[string]any)
			if errObj == nil || errObj["kind"] != "api" {
				t.Fatalf("kind must agree with exit 1 (want \"api\"), got: %s", errb.String())
			}
		})
	}
}

func TestUnknownSubcommandIsUsage(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "product", "frobnicate")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", res.result.ExitCode)
	}
}

func TestCustomerCreateRequiresEmail(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, 200, `{}`)
	defer srv.Close()

	res := runAgainst(t, srv, "customer", "create", "--first-name", "A")
	if res.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (email required)", res.result.ExitCode)
	}
}

func TestOrderUpdateForwardsTagsAndNote(t *testing.T) {
	var reqs []capturedRequest
	body := `{"data":{"orderUpdate":{"order":{"id":"gid://shopify/Order/9","note":"vip"},"userErrors":[]}}}`
	srv := newServer(t, &reqs, 200, body)
	defer srv.Close()

	res := runAgainst(t, srv, "order", "update", "9", "--note", "vip", "--tag", "priority", "--tag", "reviewed")
	if res.result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.result.ExitCode, res.stderr)
	}
	_, vars := bodyOf(t, reqs[0].Body)
	input, _ := vars["input"].(map[string]any)
	if input["note"] != "vip" {
		t.Errorf("note = %v", input["note"])
	}
	tags, _ := input["tags"].([]any)
	if len(tags) != 2 || tags[0] != "priority" {
		t.Errorf("tags = %v", input["tags"])
	}
}

func TestNewCommandTreeTraversable(t *testing.T) {
	// The design-318 dry-run seam must build without credentials.
	root := (&Service{}).NewCommandTree()
	if root == nil {
		t.Fatal("NewCommandTree returned nil")
	}
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"product", "order", "customer", "inventory", "shop", "graphql"} {
		if !names[want] {
			t.Errorf("command tree missing %q group", want)
		}
	}
}
