package segment

import (
	"strings"
	"testing"
)

const okBody = `{"data":{"ok":true}}`

// listBody mirrors the Segment list envelope (data + full pagination block,
// including previous) so the passthrough test can assert previous survives.
const listBody = `{"data":{"sources":[{"id":"src_1"}]},"pagination":{"current":"MA==","next":"MQ==","previous":null,"totalEntries":1}}`

// TestRouting exercises every first-class subcommand's method + path + auth
// header against the fake API, and asserts the provider body is passed through
// verbatim on stdout.
func TestRouting(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		body       string
	}{
		{"workspace get", []string{"workspace", "get"}, "GET", "/", okBody},
		{"source list", []string{"source", "list"}, "GET", "/sources", listBody},
		{"source get", []string{"source", "get", "--id", "src_1"}, "GET", "/sources/src_1", okBody},
		{"source connected-destinations", []string{"source", "connected-destinations", "--id", "src_1"}, "GET", "/sources/src_1/connected-destinations", listBody},
		{"destination list", []string{"destination", "list"}, "GET", "/destinations", listBody},
		{"destination get", []string{"destination", "get", "--id", "d1"}, "GET", "/destinations/d1", okBody},
		{"warehouse list", []string{"warehouse", "list"}, "GET", "/warehouses", listBody},
		{"warehouse get", []string{"warehouse", "get", "--id", "w1"}, "GET", "/warehouses/w1", okBody},
		{"tracking-plan list", []string{"tracking-plan", "list"}, "GET", "/tracking-plans", listBody},
		{"tracking-plan get", []string{"tracking-plan", "get", "--id", "tp1"}, "GET", "/tracking-plans/tp1", okBody},
		{"tracking-plan rules", []string{"tracking-plan", "rules", "--id", "tp1"}, "GET", "/tracking-plans/tp1/rules", listBody},
		{"function list", []string{"function", "list"}, "GET", "/functions", listBody},
		{"space list", []string{"space", "list"}, "GET", "/spaces", listBody},
		{"space audiences", []string{"space", "audiences", "--id", "sp1"}, "GET", "/spaces/sp1/audiences", listBody},
		{"iam user list", []string{"iam", "user", "list"}, "GET", "/users", listBody},
		{"iam group list", []string{"iam", "group", "list"}, "GET", "/groups", listBody},
		{"delivery events-volume", []string{"delivery", "events-volume"}, "GET", "/events/volume", okBody},
		{"delivery metrics", []string{"delivery", "metrics", "--destination-id", "d1"}, "GET", "/destinations/d1/delivery-metrics", okBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				tc.wantMethod + " " + tc.wantPath: {status: 200, body: tc.body},
			})
			defer srv.Close()

			got := run(t, srv, "tok_1", tc.args...)
			if got.result.ExitCode != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
			}
			req := findReq(reqs, tc.wantMethod, tc.wantPath)
			if req == nil {
				t.Fatalf("no %s %s request; got %+v", tc.wantMethod, tc.wantPath, reqs)
			}
			if req.Auth != "Bearer tok_1" {
				t.Errorf("Authorization = %q, want %q", req.Auth, "Bearer tok_1")
			}
			if strings.TrimSpace(got.stdout) != tc.body {
				t.Errorf("stdout = %q, want verbatim %q", got.stdout, tc.body)
			}
		})
	}
}

// TestPaginationDotNotation pins the pagination query encoding: dot notation
// (pagination.count / pagination.cursor), per the canonical Segment OpenAPI
// reference. This is the single encoding assertion; L2 against the live API is
// the final arbiter and only paginationQuery changes if it disagrees.
func TestPaginationDotNotation(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /sources": {status: 200, body: listBody}})
	defer srv.Close()

	got := run(t, srv, "tok_1", "source", "list", "--count", "3", "--cursor", "Mw==")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, "GET", "/sources")
	if req == nil {
		t.Fatal("no GET /sources request")
	}
	if v := req.Query.Get("pagination.count"); v != "3" {
		t.Errorf("pagination.count = %q, want %q", v, "3")
	}
	if v := req.Query.Get("pagination.cursor"); v != "Mw==" {
		t.Errorf("pagination.cursor = %q, want %q", v, "Mw==")
	}
}

// TestPaginationQueryHelper asserts the pagination-query helper directly so the
// encoding contract is pinned in one place, independent of any command.
func TestPaginationQueryHelper(t *testing.T) {
	q := paginationQuery(0, "")
	if len(q) != 0 {
		t.Errorf("empty pagination should emit no params, got %v", q)
	}
	q = paginationQuery(50, "abc")
	if q.Get("pagination.count") != "50" || q.Get("pagination.cursor") != "abc" {
		t.Errorf("paginationQuery(50, abc) = %v", q)
	}
}

// TestEventsVolumeConvenienceFlags maps the recipe-confirmed events-volume
// query params (granularity/startTime/endTime).
func TestEventsVolumeConvenienceFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /events/volume": {status: 200, body: okBody}})
	defer srv.Close()

	got := run(t, srv, "tok_1", "delivery", "events-volume",
		"--granularity", "HOUR", "--start", "2026-07-01T00:00:00Z", "--end", "2026-07-02T00:00:00Z")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, "GET", "/events/volume")
	if req.Query.Get("granularity") != "HOUR" {
		t.Errorf("granularity = %q", req.Query.Get("granularity"))
	}
	if req.Query.Get("startTime") != "2026-07-01T00:00:00Z" {
		t.Errorf("startTime = %q", req.Query.Get("startTime"))
	}
	if req.Query.Get("endTime") != "2026-07-02T00:00:00Z" {
		t.Errorf("endTime = %q", req.Query.Get("endTime"))
	}
}

// TestParamPassthrough asserts repeatable --param name=value query pairs reach
// the request unchanged (the escape hatch for L2-gated delivery query params).
func TestParamPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /destinations/d1/delivery-metrics": {status: 200, body: okBody}})
	defer srv.Close()

	got := run(t, srv, "tok_1", "delivery", "metrics", "--destination-id", "d1",
		"--param", "sourceId=src_1", "--param", "granularity=DAY")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, "GET", "/destinations/d1/delivery-metrics")
	if req.Query.Get("sourceId") != "src_1" || req.Query.Get("granularity") != "DAY" {
		t.Errorf("query = %v, want sourceId=src_1 & granularity=DAY", req.Query)
	}
}

// TestRequestEscapeHatch drives the raw request verb: arbitrary method + path +
// query, bearer-injected, JSON passed through verbatim.
func TestRequestEscapeHatch(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /sources": {status: 200, body: listBody}})
	defer srv.Close()

	got := run(t, srv, "tok_1", "request", "--method", "GET", "--path", "/sources", "--query", "foo=bar")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, "GET", "/sources")
	if req == nil {
		t.Fatal("no GET /sources request")
	}
	if req.Auth != "Bearer tok_1" {
		t.Errorf("Authorization = %q", req.Auth)
	}
	if req.Query.Get("foo") != "bar" {
		t.Errorf("query foo = %q, want bar", req.Query.Get("foo"))
	}
	if strings.TrimSpace(got.stdout) != listBody {
		t.Errorf("stdout = %q, want verbatim %q", got.stdout, listBody)
	}
}

// TestRequestPathNormalization strips a leading host / trailing base so an
// agent can paste a full URL or a /-prefixed path interchangeably.
func TestRequestPathNormalization(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /warehouses": {status: 200, body: okBody}})
	defer srv.Close()

	got := run(t, srv, "tok_1", "request", "--method", "get", "--path", "warehouses")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", got.result.ExitCode, got.stderr)
	}
	if findReq(reqs, "GET", "/warehouses") == nil {
		t.Fatalf("path 'warehouses' was not normalized to /warehouses; got %+v", reqs)
	}
}

// TestAPIErrorExit1 asserts a non-2xx (non-401) is a runtime error: exit 1, the
// provider message on stderr, credential NOT rejected.
func TestAPIErrorExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /sources/missing": {status: 404, body: `{"errors":[{"type":"not_found","message":"source not found"}]}`},
	})
	defer srv.Close()

	got := run(t, srv, "tok_1", "source", "get", "--id", "missing")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	if got.result.CredentialRejected {
		t.Error("a 404 must not reject the credential")
	}
	if !strings.Contains(got.stderr, "source not found") {
		t.Errorf("stderr = %q, want it to carry the provider message", got.stderr)
	}
}

// TestJSONErrorEnvelope asserts the --json error envelope shape (message, kind,
// status) for an API error.
func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /sources/missing": {status: 404, body: `{"errors":[{"type":"not_found","message":"source not found"}]}`},
	})
	defer srv.Close()

	got := run(t, srv, "tok_1", "--json", "source", "get", "--id", "missing")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	s := got.stderr
	if !strings.Contains(s, `"error"`) || !strings.Contains(s, `"kind":"api"`) || !strings.Contains(s, `"status":404`) {
		t.Errorf("json error envelope = %q, want error/kind:api/status:404", s)
	}
	if !strings.Contains(s, "source not found") {
		t.Errorf("json error envelope = %q, want provider message", s)
	}
}

// TestUnauthorizedRejectsCredential asserts a 401 both exits 1 and flags the
// credential rejected so the token gateway refresh path (design 227) triggers.
func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /": {status: 401, body: `{"errors":[{"type":"unauthorized","message":"invalid token"}]}`},
	})
	defer srv.Close()

	got := run(t, srv, "bad_tok", "workspace", "get")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	if !got.result.CredentialRejected {
		t.Error("a 401 must reject the credential")
	}
}

// TestMissingToken asserts a missing SEGMENT_TOKEN fails fast with exit 1 before
// any HTTP call.
func TestMissingToken(t *testing.T) {
	got := run(t, nil, "", "workspace", "get")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	if !strings.Contains(got.stderr, "SEGMENT_TOKEN") {
		t.Errorf("stderr = %q, want it to mention SEGMENT_TOKEN", got.stderr)
	}
}

// TestUsageErrorExit2 asserts a missing required flag is a usage error (exit 2),
// distinct from a runtime API error (exit 1).
func TestUsageErrorExit2(t *testing.T) {
	got := run(t, nil, "tok_1", "source", "get")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for a missing required flag", got.result.ExitCode)
	}
}

// TestUnknownSubcommandExit2 asserts an unknown subcommand is a usage error.
func TestUnknownSubcommandExit2(t *testing.T) {
	got := run(t, nil, "tok_1", "source", "bogus")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for an unknown subcommand", got.result.ExitCode)
	}
}

// TestGroupHelpNoArgs asserts a bare resource group prints help and exits 0
// (runnable group), rather than a false-success no-op.
func TestGroupHelpNoArgs(t *testing.T) {
	got := run(t, nil, "tok_1", "source")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 for a bare group (help)", got.result.ExitCode)
	}
}
