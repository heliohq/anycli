package paperform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- forms ---------------------------------------------------------------

func TestFormList(t *testing.T) {
	var reqs []capturedRequest
	body := `{"status":"ok","results":{"forms":[{"id":"abc","title":"Survey"}]},"total":1,"has_more":false}`
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 200, body: body}})
	defer srv.Close()

	out, errOut, res := run(t, srv, "form", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	if strings.TrimSpace(out) != body {
		t.Errorf("stdout passthrough mismatch:\n got %q\nwant %q", out, body)
	}
	r := findReq(reqs, http.MethodGet, "/v1/forms")
	if r == nil {
		t.Fatalf("no GET /v1/forms request; got %+v", reqs)
	}
	if r.Auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", r.Auth)
	}
}

func TestFormListPaginationFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	_, errOut, res := run(t, srv, "form", "list",
		"--limit", "50", "--skip", "10", "--sort", "ASC",
		"--after-id", "aid", "--before-id", "bid",
		"--after-date", "2026-01-01", "--before-date", "2026-02-01",
		"--search", "hello", "--search-fields", "title,slug")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	r := findReq(reqs, http.MethodGet, "/v1/forms")
	if r == nil {
		t.Fatalf("no GET /v1/forms; got %+v", reqs)
	}
	q := r.Query
	checks := map[string]string{
		"limit": "50", "skip": "10", "sort": "ASC",
		"after_id": "aid", "before_id": "bid",
		"after_date": "2026-01-01", "before_date": "2026-02-01",
		"search": "hello", "search_fields": "title,slug",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestFormListOmitsUnsetPaginationFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 200, body: `{}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "form", "list"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	r := findReq(reqs, http.MethodGet, "/v1/forms")
	if r == nil {
		t.Fatal("no request")
	}
	for _, k := range []string{"limit", "skip", "sort", "after_id", "before_id", "search"} {
		if _, ok := r.Query[k]; ok {
			t.Errorf("unset flag %q should not appear in query, got %q", k, r.Query.Get(k))
		}
	}
}

func TestFormGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/my-form": {status: 200, body: `{"status":"ok","results":{"form":{"id":"my-form"}}}`}})
	defer srv.Close()

	_, errOut, res := run(t, srv, "form", "get", "--form", "my-form")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/my-form") == nil {
		t.Fatalf("no GET /v1/forms/my-form; got %+v", reqs)
	}
}

func TestFormGetRequiresForm(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := run(t, srv, "form", "get")
	if res.ExitCode != 2 {
		t.Fatalf("missing --form should be a usage error (exit 2), got %d", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made on a usage error, got %+v", reqs)
	}
}

// --- fields --------------------------------------------------------------

func TestFieldList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/fields": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "field", "list", "--form", "f1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/fields") == nil {
		t.Fatalf("no GET /v1/forms/f1/fields; got %+v", reqs)
	}
}

func TestFieldGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/fields/email": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "field", "get", "--form", "f1", "--key", "email"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/fields/email") == nil {
		t.Fatalf("no GET /v1/forms/f1/fields/email; got %+v", reqs)
	}
}

// --- submissions ---------------------------------------------------------

func TestSubmissionList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/submissions": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "submission", "list", "--form", "f1", "--limit", "5"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	r := findReq(reqs, http.MethodGet, "/v1/forms/f1/submissions")
	if r == nil {
		t.Fatalf("no GET /v1/forms/f1/submissions; got %+v", reqs)
	}
	if r.Query.Get("limit") != "5" {
		t.Errorf("limit = %q, want 5", r.Query.Get("limit"))
	}
}

func TestSubmissionGetWithForm(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/submissions/s1": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "submission", "get", "--id", "s1", "--form", "f1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/submissions/s1") == nil {
		t.Fatalf("no GET /v1/forms/f1/submissions/s1; got %+v", reqs)
	}
}

func TestSubmissionGetWithoutForm(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/submissions/s1": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "submission", "get", "--id", "s1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/submissions/s1") == nil {
		t.Fatalf("no GET /v1/submissions/s1; got %+v", reqs)
	}
}

func TestSubmissionGetRequiresID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	if _, _, res := run(t, srv, "submission", "get"); res.ExitCode != 2 {
		t.Fatalf("missing --id should be exit 2, got %d", res.ExitCode)
	}
}

func TestPartialSubmissionList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/partial-submissions": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "partial-submission", "list", "--form", "f1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/partial-submissions") == nil {
		t.Fatalf("no GET /v1/forms/f1/partial-submissions; got %+v", reqs)
	}
}

func TestPartialSubmissionGetWithoutForm(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/partial-submissions/p1": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "partial-submission", "get", "--id", "p1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/partial-submissions/p1") == nil {
		t.Fatalf("no GET /v1/partial-submissions/p1; got %+v", reqs)
	}
}

// --- spaces & commerce ---------------------------------------------------

func TestSpaceList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/spaces": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "space", "list"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/spaces") == nil {
		t.Fatalf("no GET /v1/spaces; got %+v", reqs)
	}
}

func TestSpaceGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/spaces/sp1": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "space", "get", "--id", "sp1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/spaces/sp1") == nil {
		t.Fatalf("no GET /v1/spaces/sp1; got %+v", reqs)
	}
}

func TestSpaceForms(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/spaces/sp1/forms": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "space", "forms", "--id", "sp1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/spaces/sp1/forms") == nil {
		t.Fatalf("no GET /v1/spaces/sp1/forms; got %+v", reqs)
	}
}

func TestProductList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/products": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "product", "list", "--form", "f1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/products") == nil {
		t.Fatalf("no GET /v1/forms/f1/products; got %+v", reqs)
	}
}

func TestCouponList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/coupons": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "coupon", "list", "--form", "f1"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/coupons") == nil {
		t.Fatalf("no GET /v1/forms/f1/coupons; got %+v", reqs)
	}
}

func TestCouponGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms/f1/coupons/SAVE10": {status: 200, body: `{"status":"ok"}`}})
	defer srv.Close()

	if _, e, res := run(t, srv, "coupon", "get", "--form", "f1", "--code", "SAVE10"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, e)
	}
	if findReq(reqs, http.MethodGet, "/v1/forms/f1/coupons/SAVE10") == nil {
		t.Fatalf("no GET /v1/forms/f1/coupons/SAVE10; got %+v", reqs)
	}
}

// --- errors & auth -------------------------------------------------------

func TestMissingAPIKey(t *testing.T) {
	var out, errBuf writeBuffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"form", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("missing key should be exit 1, got %d", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PAPERFORM_API_KEY") {
		t.Errorf("stderr should mention the env var, got %q", errBuf.String())
	}
}

func TestAPIErrorJSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 500, body: `{"status":"error","message":"boom"}`}})
	defer srv.Close()

	_, errOut, res := run(t, srv, "form", "list", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("API 500 should be exit 1, got %d", res.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, errOut)
	}
	if env.Error.Status != 500 {
		t.Errorf("envelope status = %d, want 500", env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "boom") {
		t.Errorf("envelope message = %q, want it to carry the API message", env.Error.Message)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 401, body: `{"status":"error","message":"invalid key"}`}})
	defer srv.Close()

	_, _, res := run(t, srv, "form", "list")
	if res.ExitCode != 1 {
		t.Fatalf("401 should be exit 1, got %d", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
}

func TestRateLimitSurfacesRetryAfter(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /v1/forms": {status: 429, body: `{"status":"error","message":"slow down"}`, headers: map[string]string{"Retry-After": "42"}}})
	defer srv.Close()

	_, errOut, res := run(t, srv, "form", "list")
	if res.ExitCode != 1 {
		t.Fatalf("429 should be exit 1, got %d", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("429 must not reject the credential")
	}
	if !strings.Contains(errOut, "42") {
		t.Errorf("stderr should surface Retry-After (42), got %q", errOut)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	if _, _, res := run(t, srv, "form", "bogus"); res.ExitCode != 2 {
		t.Fatalf("unknown subcommand should be exit 2, got %d", res.ExitCode)
	}
}
