package braintree

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/heliohq/anycli/definitions"
)

// wantAuth is the expected Authorization header for the canonical test key
// pair: Basic base64(public_key:private_key).
var wantAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte("pubkey123:privkey_SECRET"))

func lastReq(t *testing.T, fs *fakeServer) capturedRequest {
	t.Helper()
	if len(fs.reqs) == 0 {
		t.Fatal("no request reached the fake server")
	}
	return fs.reqs[len(fs.reqs)-1]
}

// assertCommonHeaders covers DESIGN §5 L1 (a)/(b): Basic auth header,
// Braintree-Version, Content-Type, and POST method.
func assertCommonHeaders(t *testing.T, req capturedRequest) {
	t.Helper()
	if req.Method != "POST" {
		t.Errorf("method = %q, want POST", req.Method)
	}
	if req.Auth != wantAuth {
		t.Errorf("Authorization = %q, want %q", req.Auth, wantAuth)
	}
	if req.Version != braintreeVersion {
		t.Errorf("Braintree-Version = %q, want %q", req.Version, braintreeVersion)
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
}

func TestPingSuccess(t *testing.T) {
	fs := newFakeServer(t, 200, `{"data":{"ping":"pong"}}`)
	out, _, res := fs.run(t, true, "ping")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	assertCommonHeaders(t, lastReq(t, fs))
	if !strings.Contains(out, `"result":"pong"`) {
		t.Errorf("ping output = %q, want result pong", out)
	}
}

// TestResolveBaseURL covers L1 (c): the host is selected by
// BRAINTREE_ENVIRONMENT, and any other value is rejected locally.
func TestResolveBaseURL(t *testing.T) {
	cases := []struct {
		env     string
		want    string
		wantErr bool
	}{
		{"sandbox", sandboxHost, false},
		{"production", productionHost, false},
		{"", "", true},
		{"staging", "", true},
	}
	for _, c := range cases {
		got, err := resolveBaseURL(c.env)
		if c.wantErr {
			if err == nil {
				t.Errorf("resolveBaseURL(%q) err = nil, want error", c.env)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveBaseURL(%q) err = %v", c.env, err)
		}
		if got != c.want {
			t.Errorf("resolveBaseURL(%q) = %q, want %q", c.env, got, c.want)
		}
	}
}

// TestBadEnvironmentRejectedNoNetwork proves an invalid environment fails at
// exit 1 without any BaseURL override (host derivation path).
func TestBadEnvironmentRejectedNoNetwork(t *testing.T) {
	var svc Service
	env := testEnv()
	env[EnvEnvironment] = "staging"
	// No BaseURL: the environment must be consulted and rejected before any
	// network call. Capture streams.
	out := &strings.Builder{}
	errb := &strings.Builder{}
	svc.Out = out
	svc.Err = errb
	res, _ := svc.Execute(nil, []string{"ping"}, env)
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errb.String(), "BRAINTREE_ENVIRONMENT") {
		t.Errorf("stderr = %q, want BRAINTREE_ENVIRONMENT guidance", errb.String())
	}
}

func TestMissingCredentials(t *testing.T) {
	var svc Service
	errb := &strings.Builder{}
	svc.Err = errb
	svc.Out = &strings.Builder{}
	res, _ := svc.Execute(nil, []string{"ping"}, map[string]string{EnvEnvironment: "sandbox"})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
}

// TestTransactionSearchFlatten covers L1 (f): edges[].node → items + page_info.
func TestTransactionSearchFlatten(t *testing.T) {
	body := `{"data":{"search":{"transactions":{
      "pageInfo":{"hasNextPage":true,"endCursor":"CURSOR2"},
      "edges":[{"node":{"id":"txn_1","status":"SETTLED"}},{"node":{"id":"txn_2","status":"AUTHORIZED"}}]
    }}}}`
	fs := newFakeServer(t, 200, body)
	out, _, res := fs.run(t, true, "transaction", "search", "--status", "settled", "--first", "10")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := lastReq(t, fs)
	assertCommonHeaders(t, req)
	if !strings.Contains(req.Query, "transactions(input:") {
		t.Errorf("query missing transactions search: %q", req.Query)
	}
	// status flag upper-cased into an { in: [...] } matcher.
	input, _ := req.Variables["input"].(map[string]any)
	status, _ := input["status"].(map[string]any)
	inList, _ := status["in"].([]any)
	if len(inList) != 1 || inList[0] != "SETTLED" {
		t.Errorf("status matcher = %v, want in:[SETTLED]", status)
	}
	for _, want := range []string{`"items"`, `"txn_1"`, `"txn_2"`, `"has_next_page":true`, `"end_cursor":"CURSOR2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// TestVoidAndReverseMapDistinctMutations covers L1 (d): `void` emits
// voidTransaction and `reverse` emits reverseTransaction — never swapped.
func TestVoidAndReverseMapDistinctMutations(t *testing.T) {
	t.Run("void", func(t *testing.T) {
		fs := newFakeServer(t, 200, `{"data":{"voidTransaction":{"transaction":{"id":"txn_1","status":"VOIDED"}}}}`)
		_, _, res := fs.run(t, true, "transaction", "void", "txn_1")
		if res.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0", res.ExitCode)
		}
		req := lastReq(t, fs)
		if !strings.Contains(req.Query, "voidTransaction(input:") {
			t.Errorf("void query = %q, want voidTransaction", req.Query)
		}
		if strings.Contains(req.Query, "reverseTransaction") {
			t.Errorf("void must NOT emit reverseTransaction: %q", req.Query)
		}
	})
	t.Run("reverse", func(t *testing.T) {
		fs := newFakeServer(t, 200, `{"data":{"reverseTransaction":{"reversal":{"__typename":"Refund","id":"rf_1","status":"SETTLING"}}}}`)
		_, _, res := fs.run(t, true, "transaction", "reverse", "txn_1")
		if res.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0", res.ExitCode)
		}
		req := lastReq(t, fs)
		if !strings.Contains(req.Query, "reverseTransaction(input:") {
			t.Errorf("reverse query = %q, want reverseTransaction", req.Query)
		}
		if strings.Contains(req.Query, "voidTransaction") {
			t.Errorf("reverse must NOT emit voidTransaction: %q", req.Query)
		}
	})
}

func TestRefundInputShape(t *testing.T) {
	fs := newFakeServer(t, 200, `{"data":{"refundTransaction":{"refund":{"id":"rf_1","status":"SETTLING"}}}}`)
	_, _, res := fs.run(t, true, "transaction", "refund", "txn_1", "--amount", "5.00", "--order-id", "ord_9")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := lastReq(t, fs)
	if !strings.Contains(req.Query, "refundTransaction(input:") {
		t.Errorf("refund query = %q", req.Query)
	}
	input, _ := req.Variables["input"].(map[string]any)
	if input["transactionId"] != "txn_1" {
		t.Errorf("transactionId = %v, want txn_1", input["transactionId"])
	}
	refund, _ := input["refund"].(map[string]any)
	if refund["amount"] != "5.00" || refund["orderId"] != "ord_9" {
		t.Errorf("refund matcher = %v, want amount/orderId", refund)
	}
}

// TestGraphQLErrorsMapToExit1 covers L1 (e): a non-empty errors[] under HTTP
// 200 becomes an apiError at exit 1, surfacing message + errorClass.
func TestGraphQLErrorsMapToExit1(t *testing.T) {
	body := `{"errors":[{"message":"Transaction cannot be voided in its current state","extensions":{"errorClass":"VALIDATION"}}]}`
	fs := newFakeServer(t, 200, body)
	_, errOut, res := fs.run(t, true, "transaction", "void", "txn_1")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errOut, "cannot be voided") || !strings.Contains(errOut, "VALIDATION") {
		t.Errorf("stderr = %q, want message + errorClass", errOut)
	}
}

// TestCredentialRejection covers a 401 → credential rejected classification.
func TestCredentialRejection(t *testing.T) {
	fs := newFakeServer(t, 401, `{"error":"unauthorized"}`)
	_, _, res := fs.run(t, false, "ping")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
}

// TestSecretsNeverPrinted covers L1 (g): neither the private nor public key
// appears in stdout or stderr, even on error.
func TestSecretsNeverPrinted(t *testing.T) {
	fs := newFakeServer(t, 200, `{"errors":[{"message":"boom"}]}`)
	out, errOut, _ := fs.run(t, true, "ping")
	for _, secret := range []string{"privkey_SECRET", "pubkey123", wantAuth} {
		if strings.Contains(out, secret) || strings.Contains(errOut, secret) {
			t.Errorf("secret %q leaked into output", secret)
		}
	}
}

// TestQueryRejectsMutationLocally covers L1 (h): a mutation supplied to the
// read-only `query` passthrough is rejected at exit 2 with NO HTTP request,
// while a read query passes through.
func TestQueryRejectsMutationLocally(t *testing.T) {
	t.Run("mutation rejected, no request", func(t *testing.T) {
		fs := newFakeServer(t, 200, `{"data":{}}`)
		_, errOut, res := fs.run(t, true, "query", "mutation { refundTransaction(input: {transactionId: \"x\"}) { refund { id } } }")
		if res.ExitCode != 2 {
			t.Fatalf("exit = %d, want 2", res.ExitCode)
		}
		if len(fs.reqs) != 0 {
			t.Errorf("mutation rejection must not issue an HTTP request, got %d", len(fs.reqs))
		}
		if !strings.Contains(errOut, "read-only") {
			t.Errorf("stderr = %q, want read-only rejection", errOut)
		}
	})
	t.Run("read query passes through", func(t *testing.T) {
		fs := newFakeServer(t, 200, `{"data":{"ping":"pong"}}`)
		out, _, res := fs.run(t, true, "query", "query { ping }")
		if res.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0", res.ExitCode)
		}
		if len(fs.reqs) != 1 {
			t.Fatalf("read query should issue exactly one request, got %d", len(fs.reqs))
		}
		if !strings.Contains(out, "pong") {
			t.Errorf("query output = %q", out)
		}
	})
	t.Run("anonymous selection is a read", func(t *testing.T) {
		fs := newFakeServer(t, 200, `{"data":{"ping":"pong"}}`)
		_, _, res := fs.run(t, true, "query", "  # comment\n { ping }")
		if res.ExitCode != 0 {
			t.Fatalf("anonymous selection exit = %d, want 0", res.ExitCode)
		}
	})
}

func TestIsMutation(t *testing.T) {
	cases := []struct {
		doc  string
		want bool
	}{
		{"mutation { x }", true},
		{"  mutation Foo { x }", true},
		{"# lead comment\nmutation { x }", true},
		{"query { ping }", false},
		{"{ ping }", false},
		{"  query Foo { ping }", false},
		{"subscription { x }", false},
		{",, { ping }", false},
	}
	for _, c := range cases {
		if got := isMutation(c.doc); got != c.want {
			t.Errorf("isMutation(%q) = %v, want %v", c.doc, got, c.want)
		}
	}
}

// TestNodeGetNotFound proves a null node maps to a runtime not-found (exit 1).
func TestNodeGetNotFound(t *testing.T) {
	fs := newFakeServer(t, 200, `{"data":{"node":null}}`)
	_, _, res := fs.run(t, true, "transaction", "get", "txn_missing")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
}

// TestDefinitionLoads proves the bundled definition parses and injects the four
// credential fields as the expected env vars.
func TestDefinitionLoads(t *testing.T) {
	def, err := definitions.LoadBundled("braintree")
	if err != nil {
		t.Fatalf("LoadBundled: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("type = %q, want service", def.Type)
	}
	wantEnv := map[string]string{
		"merchant_id": EnvMerchantID,
		"public_key":  EnvPublicKey,
		"private_key": EnvPrivateKey,
		"environment": EnvEnvironment,
	}
	got := map[string]string{}
	for _, c := range def.Auth.Credentials {
		got[c.Source.Field] = c.Inject.EnvVar
	}
	for field, env := range wantEnv {
		if got[field] != env {
			t.Errorf("field %q injects %q, want %q", field, got[field], env)
		}
	}
}

// TestUsageErrorExit2 proves an unknown subcommand is a usage error (exit 2).
func TestUsageErrorExit2(t *testing.T) {
	fs := newFakeServer(t, 200, `{"data":{}}`)
	_, _, res := fs.run(t, false, "bogus")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
}
