package pipedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- credential guardrails -------------------------------------------------

func TestExecute_MissingAccessToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"deal", "list"},
		map[string]string{EnvAPIDomain: "https://acme.pipedrive.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PIPEDRIVE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestExecute_MissingAPIDomain(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"deal", "list"},
		map[string]string{EnvAccessToken: "tok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PIPEDRIVE_API_DOMAIN is not set") {
		t.Errorf("stderr = %q, want missing-api-domain message", errBuf.String())
	}
}

func TestExecute_MalformedAPIDomain(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"deal", "list"},
		map[string]string{EnvAccessToken: "tok", EnvAPIDomain: "acme.pipedrive.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1 (no fallback host)", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "PIPEDRIVE_API_DOMAIN") {
		t.Errorf("stderr = %q, want api-domain validation message", errBuf.String())
	}
}

func TestExecute_MissingAccessToken_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"deal", "list", "--json"},
		map[string]string{EnvAPIDomain: "https://acme.pipedrive.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "PIPEDRIVE_ACCESS_TOKEN is not set") {
		t.Errorf("envelope = %+v, want kind=usage missing-token", env.Error)
	}
}

// --- base URL, auth header, verbatim envelope ------------------------------

func TestDealList_AuthAndBaseURLAndVerbatim(t *testing.T) {
	var got capturedRequest
	const envelope = `{"success":true,"data":[{"id":1,"title":"Acme deal"}],"additional_data":{"next_cursor":"abc"}}`
	srv := newServer(t, http.StatusOK, envelope, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, fullEnv(srv), "deal", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Auth != "Bearer seed-access-token" {
		t.Errorf("Authorization = %q, want Bearer seed-access-token", got.Auth)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v2/deals" {
		t.Errorf("request = %s %s, want GET /api/v2/deals", got.Method, got.Path)
	}
	if strings.TrimSpace(stdout) != envelope {
		t.Errorf("stdout = %q, want verbatim envelope %q", stdout, envelope)
	}
}

func TestDealList_CursorAndLimitPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"deal", "list", "--cursor", "CURSOR1", "--limit", "50",
		"--status", "open", "--person-id", "7")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Query.Get("cursor") != "CURSOR1" {
		t.Errorf("cursor = %q, want CURSOR1", got.Query.Get("cursor"))
	}
	if got.Query.Get("limit") != "50" {
		t.Errorf("limit = %q, want 50", got.Query.Get("limit"))
	}
	if got.Query.Get("status") != "open" {
		t.Errorf("status = %q, want open", got.Query.Get("status"))
	}
	if got.Query.Get("person_id") != "7" {
		t.Errorf("person_id = %q, want 7", got.Query.Get("person_id"))
	}
}

// --- v2 write semantics: create POST, update PATCH -------------------------

func TestDealCreate_POSTBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"success":true,"data":{"id":9}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"deal", "create", "--title", "Big deal", "--value", "1000", "--currency", "USD")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v2/deals" {
		t.Errorf("request = %s %s, want POST /api/v2/deals", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, got.Body)
	}
	if body["title"] != "Big deal" {
		t.Errorf("body.title = %v, want Big deal", body["title"])
	}
	if body["value"] != float64(1000) {
		t.Errorf("body.value = %v (%T), want number 1000", body["value"], body["value"])
	}
	if body["currency"] != "USD" {
		t.Errorf("body.currency = %v, want USD", body["currency"])
	}
}

func TestDealUpdate_PATCHWithID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"id":9}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"deal", "update", "9", "--stage-id", "3", "--status", "won")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPatch || got.Path != "/api/v2/deals/9" {
		t.Errorf("request = %s %s, want PATCH /api/v2/deals/9", got.Method, got.Path)
	}
	var body map[string]any
	_ = json.Unmarshal(got.Body, &body)
	if body["stage_id"] != float64(3) {
		t.Errorf("body.stage_id = %v, want 3", body["stage_id"])
	}
	if body["status"] != "won" {
		t.Errorf("body.status = %v, want won", body["status"])
	}
}

func TestDealCreate_DataEscapeHatchMerges(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"success":true,"data":{"id":9}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"deal", "create", "--data", `{"title":"Raw","visible_to":"3"}`, "--value", "42")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var body map[string]any
	_ = json.Unmarshal(got.Body, &body)
	if body["title"] != "Raw" {
		t.Errorf("body.title = %v, want Raw (from --data)", body["title"])
	}
	if body["visible_to"] != "3" {
		t.Errorf("body.visible_to = %v, want 3 (from --data)", body["visible_to"])
	}
	if body["value"] != float64(42) {
		t.Errorf("body.value = %v, want typed flag to overlay --data", body["value"])
	}
}

func TestDealSearch_V2Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"items":[]}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv), "deal", "search", "--term", "Acme")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/api/v2/deals/search" {
		t.Errorf("path = %q, want /api/v2/deals/search", got.Path)
	}
	if got.Query.Get("term") != "Acme" {
		t.Errorf("term = %q, want Acme", got.Query.Get("term"))
	}
}

// --- v2 read-only + activity delete ----------------------------------------

func TestPipelineList_V2ReadOnly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "pipeline", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/v2/pipelines" {
		t.Errorf("path = %q, want /api/v2/pipelines", got.Path)
	}
}

func TestActivityDelete_V2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"id":5}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "activity", "delete", "5")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/api/v2/activities/5" {
		t.Errorf("request = %s %s, want DELETE /api/v2/activities/5", got.Method, got.Path)
	}
}

// --- v1-only surfaces: leads, notes, users ---------------------------------

func TestLeadList_V1PathAndOffsetPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv), "lead", "list", "--start", "20", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/api/v1/leads" {
		t.Errorf("path = %q, want /api/v1/leads", got.Path)
	}
	if got.Query.Get("start") != "20" || got.Query.Get("limit") != "10" {
		t.Errorf("pagination = start:%q limit:%q, want 20/10", got.Query.Get("start"), got.Query.Get("limit"))
	}
}

func TestNoteAdd_V1POST(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"success":true,"data":{"id":1}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"note", "add", "--content", "Called the lead", "--deal-id", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/notes" {
		t.Errorf("request = %s %s, want POST /api/v1/notes", got.Method, got.Path)
	}
	var body map[string]any
	_ = json.Unmarshal(got.Body, &body)
	if body["content"] != "Called the lead" {
		t.Errorf("body.content = %v, want Called the lead", body["content"])
	}
	if body["deal_id"] != float64(9) {
		t.Errorf("body.deal_id = %v, want 9", body["deal_id"])
	}
}

func TestNoteUpdate_V1PUT(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"id":1}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "note", "update", "1", "--content", "Edited")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/api/v1/notes/1" {
		t.Errorf("request = %s %s, want PUT /api/v1/notes/1", got.Method, got.Path)
	}
}

func TestUserMe_V1Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"id":1,"name":"Me"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "user", "me")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/api/v1/users/me" {
		t.Errorf("path = %q, want /api/v1/users/me", got.Path)
	}
}

// --- cross-entity search (itemSearch) --------------------------------------

func TestSearch_ItemSearchV2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"data":{"items":[]}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv),
		"search", "--term", "Acme", "--types", "deal,person", "--limit", "20")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/api/v2/itemSearch" {
		t.Errorf("path = %q, want /api/v2/itemSearch", got.Path)
	}
	if got.Query.Get("term") != "Acme" {
		t.Errorf("term = %q, want Acme", got.Query.Get("term"))
	}
	if got.Query.Get("item_types") != "deal,person" {
		t.Errorf("item_types = %q, want deal,person", got.Query.Get("item_types"))
	}
}

// --- error rendering + classification --------------------------------------

func TestAPIError_PlainAndExit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest,
		`{"success":false,"error":"Bad request","error_info":"See docs","data":null}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, fullEnv(srv), "deal", "get", "1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "Bad request") {
		t.Errorf("stderr = %q, want provider error message", stderr)
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden,
		`{"success":false,"error":"Forbidden","error_info":"scope","data":null}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv), "deal", "get", "1", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusForbidden {
		t.Errorf("envelope = %+v, want kind=api status=403", env.Error)
	}
	if !strings.Contains(env.Error.Message, "Forbidden") {
		t.Errorf("message = %q, want provider error", env.Error.Message)
	}
}

func TestUnauthorized_ClassifiedAsCredentialRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized,
		`{"success":false,"error":"unauthorized","error_info":"","data":null}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, fullEnv(srv), "deal", "list")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on HTTP 401")
	}
}

func TestNonCredentialError_NotRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests,
		`{"success":false,"error":"rate limit","error_info":"","data":null}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, fullEnv(srv), "deal", "list")
	if result.CredentialRejected {
		t.Errorf("CredentialRejected = true on 429, want false")
	}
}

// --- usage errors → exit 2 -------------------------------------------------

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(srv), "deal", "frobnicate")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestGetMissingID_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "deal", "get")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for missing required id arg", code)
	}
}

func TestSearchMissingTerm_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(srv), "search")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 when required --term is missing", code)
	}
}

// --- multi-call sequence: read stages, then move a deal --------------------

// TestStageListThenDealMove exercises the realistic two-step flow an agent
// follows to move a deal: list the pipeline's stages, then PATCH the deal onto
// one of them. Each command is a separate Execute call against one multi-route
// fake server, so this asserts both requests were routed, paginated, and
// authorized correctly — coverage the single-response newServer harness cannot
// express.
func TestStageListThenDealMove(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v2/stages":     {status: http.StatusOK, body: `{"success":true,"data":[{"id":7,"pipeline_id":3}]}`},
		"PATCH /api/v2/deals/42": {status: http.StatusOK, body: `{"success":true,"data":{"id":42,"stage_id":7}}`},
	})
	defer srv.Close()

	if code, _, stderr := run(t, srv, fullEnv(srv), "stage", "list", "--pipeline-id", "3"); code != 0 {
		t.Fatalf("stage list exit = %d, stderr = %q", code, stderr)
	}
	if code, _, stderr := run(t, srv, fullEnv(srv), "deal", "update", "42", "--stage-id", "7"); code != 0 {
		t.Fatalf("deal update exit = %d, stderr = %q", code, stderr)
	}

	stageReq := findReq(reqs, http.MethodGet, "/api/v2/stages")
	if stageReq == nil {
		t.Fatalf("no GET /api/v2/stages recorded; saw %+v", reqs)
	}
	if stageReq.Query.Get("pipeline_id") != "3" {
		t.Errorf("stage list pipeline_id = %q, want 3", stageReq.Query.Get("pipeline_id"))
	}
	if stageReq.Auth != "Bearer seed-access-token" {
		t.Errorf("stage list Authorization = %q, want Bearer seed-access-token", stageReq.Auth)
	}

	moveReq := findReq(reqs, http.MethodPatch, "/api/v2/deals/42")
	if moveReq == nil {
		t.Fatalf("no PATCH /api/v2/deals/42 recorded; saw %+v", reqs)
	}
	if moveReq.Auth != "Bearer seed-access-token" {
		t.Errorf("deal move Authorization = %q, want Bearer seed-access-token", moveReq.Auth)
	}
	var body map[string]any
	if err := json.Unmarshal(moveReq.Body, &body); err != nil {
		t.Fatalf("deal move body not JSON: %v (%s)", err, moveReq.Body)
	}
	if body["stage_id"] != float64(7) {
		t.Errorf("deal move body.stage_id = %v, want 7", body["stage_id"])
	}
}
