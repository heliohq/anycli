package sage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// TestListSendsBearerAndAcceptAndNoBusiness proves every call carries the
// Bearer token and JSON Accept, and that omitting --business omits the
// X-Business header (Sage falls back to the lead business).
func TestListSendsBearerAndAcceptAndNoBusiness(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /businesses": {status: 200, body: `{"$items":[{"id":"b1"}],"$total":1}`},
	})
	defer srv.Close()

	out, errOut, res := run(t, srv, "business", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/businesses")
	if req == nil {
		t.Fatal("no GET /businesses recorded")
	}
	if req.Auth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q, want Bearer tok-abc", req.Auth)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
	if req.Business != "" {
		t.Errorf("X-Business = %q, want empty (lead business fallback)", req.Business)
	}
	if !strings.Contains(out, "$items") {
		t.Errorf("stdout does not carry the Sage list envelope verbatim: %q", out)
	}
}

// TestBusinessFlagSetsXBusinessHeader proves the global --business flag maps to
// the X-Business header on the underlying request.
func TestBusinessFlagSetsXBusinessHeader(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {status: 200, body: `{"$items":[]}`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "contact", "list", "--business", "biz-123")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/contacts")
	if req == nil {
		t.Fatal("no GET /contacts recorded")
	}
	if req.Business != "biz-123" {
		t.Errorf("X-Business = %q, want biz-123", req.Business)
	}
}

// TestMissingTokenExitsOne proves a missing SAGE_ACCESS_TOKEN fails fast at
// exit 1 before any HTTP call.
func TestMissingTokenExitsOne(t *testing.T) {
	var out, errOut writerBuf
	svc := &Service{Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"business", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "SAGE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errOut.String())
	}
}

// TestAPIErrorExitsOneAndSurfacesMessage proves a Sage non-2xx maps to exit 1
// and surfaces the parsed error message (array-of-objects error dialect).
func TestAPIErrorExitsOneAndSurfacesMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /contacts/c-missing": {status: 404, body: `[{"$severity":"error","$message":"Contact not found","$dataCode":"ResourceNotFound"}]`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "contact", "get", "c-missing")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (stderr=%q)", res.ExitCode, errOut)
	}
	if !strings.Contains(errOut, "Contact not found") || !strings.Contains(errOut, "ResourceNotFound") {
		t.Errorf("stderr = %q, want the parsed Sage error message + data code", errOut)
	}
	if !strings.Contains(errOut, "404") {
		t.Errorf("stderr = %q, want the HTTP status", errOut)
	}
}

// TestUnauthorizedIsCredentialRejected proves a 401 is classified as a
// credential rejection so the Helio token gateway triggers refresh/re-consent.
func TestUnauthorizedIsCredentialRejected(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /businesses": {status: 401, body: `[{"$severity":"error","$message":"Unauthorized"}]`},
	})
	defer srv.Close()

	_, _, res := run(t, srv, "business", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 was not classified as a credential rejection")
	}
}

// TestUsageErrorExitsTwo proves a parse/usage error (unknown subcommand) maps
// to exit 2, not 1.
func TestUsageErrorExitsTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := run(t, srv, "contact", "frobnicate")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("a usage error must not reach the API; got %d requests", len(reqs))
	}
}

// TestMissingBodyExitsTwo proves `create` without --body is a usage error
// (exit 2) that never reaches the API.
func TestMissingBodyExitsTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := run(t, srv, "contact", "create")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("missing --body must not reach the API; got %d requests", len(reqs))
	}
}

// TestJSONErrorEnvelope proves --json renders the structured error envelope on
// stderr with kind=api and the HTTP status for an API failure.
func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /businesses": {status: 403, body: `[{"$message":"Forbidden"}]`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "business", "list", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, errOut)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != 403 {
		t.Errorf("status = %d, want 403", env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "Forbidden") {
		t.Errorf("message = %q, want it to carry Forbidden", env.Error.Message)
	}
}

// TestNewCommandTreeDryRun proves the design-318 seam builds the full tree with
// an empty token without executing any RunE.
func TestNewCommandTreeDryRun(t *testing.T) {
	root := (&Service{}).NewCommandTree()
	if root == nil {
		t.Fatal("NewCommandTree returned nil")
	}
	if root.Use != "sage" {
		t.Errorf("root Use = %q, want sage", root.Use)
	}
	// Spot-check a couple of expected groups exist.
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"business", "contact", "sales-invoice", "purchase-invoice", "contact-payment", "fetch"} {
		if !names[want] {
			t.Errorf("command tree missing %q group/leaf", want)
		}
	}
}

// ensure execution import is used even if some assertions are trimmed.
var _ = execution.Result{}
