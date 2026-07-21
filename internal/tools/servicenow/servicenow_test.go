package servicenow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- credential handling -----------------------------------------------------

func TestExecute_MissingInstanceURL(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(),
		[]string{"table", "query", "incident"},
		map[string]string{EnvAPIKey: "k"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvInstanceURL+" is not set") {
		t.Errorf("stderr = %q, want missing instance URL message", errBuf.String())
	}
}

func TestExecute_MissingAPIKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(),
		[]string{"table", "query", "incident"},
		map[string]string{EnvInstanceURL: "https://acme.service-now.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey+" is not set") {
		t.Errorf("stderr = %q, want missing API key message", errBuf.String())
	}
}

func TestExecute_MissingCredential_JSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, _ := svc.Execute(context.Background(),
		[]string{"table", "query", "incident", "--json"},
		map[string]string{EnvInstanceURL: "https://acme.service-now.com"})
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%q)", err, errBuf.String())
	}
	if !strings.Contains(env.Error.Message, EnvAPIKey+" is not set") {
		t.Errorf("envelope = %+v, want missing API key message", env.Error)
	}
}

// --- base URL derivation + auth header ---------------------------------------

func TestTableQuery_DerivesBaseURLAndInjectsAPIKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":[{"number":"INC0010001"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "table", "query", "incident",
		"--query", "active=true^priority=1", "--limit", "5", "--fields", "number,state",
		"--offset", "10", "--display-value", "all")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Base URL derived from SERVICENOW_INSTANCE_URL → /api/now/table/incident.
	if got.Method != http.MethodGet || got.Path != "/api/now/table/incident" {
		t.Errorf("request = %s %s, want GET /api/now/table/incident", got.Method, got.Path)
	}
	if got.APIKey != testAPIKey {
		t.Errorf("%s = %q, want %q", apiKeyHeader, got.APIKey, testAPIKey)
	}
	// sysparm_* query mapping.
	q := got.Query
	if q.Get("sysparm_query") != "active=true^priority=1" {
		t.Errorf("sysparm_query = %q", q.Get("sysparm_query"))
	}
	if q.Get("sysparm_limit") != "5" || q.Get("sysparm_offset") != "10" {
		t.Errorf("limit/offset = %q/%q, want 5/10", q.Get("sysparm_limit"), q.Get("sysparm_offset"))
	}
	if q.Get("sysparm_fields") != "number,state" {
		t.Errorf("sysparm_fields = %q", q.Get("sysparm_fields"))
	}
	if q.Get("sysparm_display_value") != "all" {
		t.Errorf("sysparm_display_value = %q", q.Get("sysparm_display_value"))
	}
	// {result} unwrapping: stdout is the bare array.
	var arr []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &arr); err != nil {
		t.Fatalf("stdout is not an unwrapped array: %v (%q)", err, stdout)
	}
	if len(arr) != 1 || arr[0]["number"] != "INC0010001" {
		t.Errorf("unwrapped result = %v", arr)
	}
}

func TestNormalizeInstanceURL(t *testing.T) {
	cases := map[string]string{
		"https://acme.service-now.com":        "https://acme.service-now.com",
		"https://acme.service-now.com/":       "https://acme.service-now.com",
		"https://acme.service-now.com/nav.do": "https://acme.service-now.com",
		"acme.service-now.com":                "https://acme.service-now.com",
		"http://127.0.0.1:8080/x?y=1":         "http://127.0.0.1:8080",
	}
	for in, want := range cases {
		got, err := normalizeInstanceURL(in)
		if err != nil {
			t.Errorf("normalizeInstanceURL(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeInstanceURL(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := normalizeInstanceURL(""); err == nil {
		t.Error("empty instance URL should error")
	}
}

// --- table get / create / update / delete ------------------------------------

func TestTableGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":{"sys_id":"abc","number":"INC1"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "table", "get", "incident", "abc123")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/now/table/incident/abc123" {
		t.Errorf("path = %q, want /api/now/table/incident/abc123", got.Path)
	}
	// get unwraps to a bare object, not an array.
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &obj); err != nil {
		t.Fatalf("stdout not a JSON object: %v (%q)", err, stdout)
	}
	if obj["sys_id"] != "abc" {
		t.Errorf("obj = %v", obj)
	}
}

func TestTableCreate_PostsJSONBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"result":{"sys_id":"new1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "table", "create", "incident",
		"--data", `{"short_description":"printer down"}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/now/table/incident" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q", got.ContentType)
	}
	if bodyMap(t, got.Body)["short_description"] != "printer down" {
		t.Errorf("body = %s", got.Body)
	}
}

func TestTableCreate_BadJSON_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "table", "create", "incident", "--data", `{not json`)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestTableUpdate_Patch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":{"sys_id":"abc"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "table", "update", "incident", "abc", "--data", `{"state":"2"}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/api/now/table/incident/abc" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if bodyMap(t, got.Body)["state"] != "2" {
		t.Errorf("body = %s", got.Body)
	}
}

func TestTableDelete_Receipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "table", "delete", "incident", "abc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", got.Method)
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
	if receipt["deleted"] != true || receipt["sys_id"] != "abc" {
		t.Errorf("receipt = %v", receipt)
	}
}

// --- incident sugar + number→sys_id lookup -----------------------------------

func TestIncidentGet_ResolvesNumberToSysID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		// lookup by number → sys_id
		"GET /api/now/table/incident": {http.StatusOK, `{"result":[{"sys_id":"SYS123"}]}`},
		// fetch by resolved sys_id
		"GET /api/now/table/incident/SYS123": {http.StatusOK, `{"result":{"sys_id":"SYS123","number":"INC0010001"}}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "incident", "get", "INC0010001")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	lookup := findReq(reqs, http.MethodGet, "/api/now/table/incident")
	if lookup == nil {
		t.Fatal("no number→sys_id lookup request recorded")
	}
	if lookup.Query.Get("sysparm_query") != "number=INC0010001" {
		t.Errorf("lookup query = %q, want number=INC0010001", lookup.Query.Get("sysparm_query"))
	}
	if lookup.Query.Get("sysparm_limit") != "1" {
		t.Errorf("lookup limit = %q, want 1", lookup.Query.Get("sysparm_limit"))
	}
	if findReq(reqs, http.MethodGet, "/api/now/table/incident/SYS123") == nil {
		t.Error("did not fetch by resolved sys_id")
	}
	var obj map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &obj)
	if obj["number"] != "INC0010001" {
		t.Errorf("stdout obj = %v", obj)
	}
}

func TestIncidentGet_SysIDPassthrough_NoLookup(t *testing.T) {
	var reqs []capturedRequest
	sysID := "0123456789abcdef0123456789abcdef" // 32 hex
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/now/table/incident/" + sysID: {http.StatusOK, `{"result":{"sys_id":"` + sysID + `"}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "incident", "get", sysID)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// A 32-hex sys_id must be used directly — no lookup query.
	if findReq(reqs, http.MethodGet, "/api/now/table/incident") != nil {
		t.Error("sys_id passthrough should not issue a number lookup")
	}
}

func TestIncidentGet_UnknownNumber_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/now/table/incident": {http.StatusOK, `{"result":[]}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "incident", "get", "INC9999999")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "no incident found") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestIncidentCreate_SetsShortDescription(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"result":{"sys_id":"n1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "incident", "create", "--short-description", "VPN down")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/now/table/incident" || got.Method != http.MethodPost {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if bodyMap(t, got.Body)["short_description"] != "VPN down" {
		t.Errorf("body = %s", got.Body)
	}
}

func TestIncidentResolve_SetsStateAndCloseFields(t *testing.T) {
	var reqs []capturedRequest
	sysID := "0123456789abcdef0123456789abcdef"
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /api/now/table/incident/" + sysID: {http.StatusOK, `{"result":{"sys_id":"` + sysID + `","state":"6"}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "incident", "resolve", sysID,
		"--close-notes", "rebooted", "--code", "Solved (Permanently)")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	patch := findReq(reqs, http.MethodPatch, "/api/now/table/incident/"+sysID)
	if patch == nil {
		t.Fatal("no resolve PATCH recorded")
	}
	b := bodyMap(t, patch.Body)
	if b["state"] != stateResolved {
		t.Errorf("state = %v, want %s", b["state"], stateResolved)
	}
	if b["close_notes"] != "rebooted" || b["close_code"] != "Solved (Permanently)" {
		t.Errorf("close fields = %v", b)
	}
}

func TestIncidentResolve_MissingCloseNotes_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "incident", "resolve", "0123456789abcdef0123456789abcdef")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "close-notes") {
		t.Errorf("stderr = %q", stderr)
	}
}

// --- whoami ------------------------------------------------------------------

func TestWhoami_VerifiesAndEchoesIdentity(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"result":[{"sys_id":"u1","user_name":"helio.int","email":"a@b.co"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "whoami")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/now/table/sys_user" {
		t.Errorf("path = %q, want /api/now/table/sys_user", got.Path)
	}
	if got.APIKey != testAPIKey {
		t.Errorf("api key not injected on whoami")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &obj); err != nil {
		t.Fatalf("stdout not an object: %v (%q)", err, stdout)
	}
	if obj["user_name"] != "helio.int" {
		t.Errorf("identity = %v", obj)
	}
}

// --- raw api verb ------------------------------------------------------------

func TestAPI_RawRequest(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":{"stats":{"count":"3"}}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "api", "GET", "/api/now/stats/incident",
		"--query", "sysparm_count=true")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/now/stats/incident" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query.Get("sysparm_count") != "true" {
		t.Errorf("query = %v", got.Query)
	}
	// api emits the raw body verbatim (no unwrapping).
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if _, ok := env["result"]; !ok {
		t.Errorf("api should emit the raw envelope, got %v", env)
	}
}

func TestAPI_NowShorthandPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "api", "GET", "/now/table/problem")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/now/table/problem" {
		t.Errorf("path = %q, want /api/now/table/problem (shorthand prefixed)", got.Path)
	}
}

func TestAPI_RejectsAuthHeaderOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "api", "GET", "/api/now/table/incident",
		"--header", "x-sn-apikey:evil")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "cannot be overridden") {
		t.Errorf("stderr = %q, want override rejection", stderr)
	}
}

// --- error rendering + credential rejection ----------------------------------

func TestAPIError_PlainAndJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest,
		`{"error":{"message":"Invalid table","detail":"bad_table not found"},"status":"failure"}`, &got)
	defer srv.Close()

	// plain
	code, _, stderr := run(t, srv, "table", "query", "bad_table")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (api error)", code)
	}
	if !strings.Contains(stderr, "Invalid table") || !strings.Contains(stderr, "HTTP 400") {
		t.Errorf("plain stderr = %q", stderr)
	}

	// json envelope
	codeJSON, _, stderrJSON := run(t, srv, "table", "query", "bad_table", "--json")
	if codeJSON != 1 {
		t.Fatalf("exit = %d, want 1", codeJSON)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderrJSON)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%q)", err, stderrJSON)
	}
	if env.Error.Status != 400 || env.Error.Detail != "bad_table not found" {
		t.Errorf("envelope = %+v", env.Error)
	}
}

func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized,
		`{"error":{"message":"User Not Authenticated","detail":"Required to provide Auth information"},"status":"failure"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "table", "query", "incident")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 should classify as a credential rejection")
	}
}

func TestUnknownSubcommand_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "table", "bogus", "incident")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", code)
	}
}
