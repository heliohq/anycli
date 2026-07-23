package kit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAccountGetIdentity(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v4/account": {200, `{"user":{"email":"c@example.com","id":9},"account":{"id":12345,"name":"Acme","plan_type":"pro","primary_email_address":"c@example.com"}}`},
	})
	defer srv.Close()

	out, errStr, res := run(t, srv.URL+"/v4", tokenEnv(), "account", "get")
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, errStr)
	}
	req := findReq(reqs, "GET", "/v4/account")
	if req == nil {
		t.Fatal("GET /v4/account not called")
	}
	if req.Auth != "Bearer T" {
		t.Fatalf("auth header = %q, want Bearer T", req.Auth)
	}
	env := decode(t, []byte(out))
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope has no data object: %s", out)
	}
	acct, ok := data["account"].(map[string]any)
	if !ok {
		t.Fatalf("data has no account: %s", out)
	}
	// account.id is an integer identity — it must survive verbatim.
	if id, _ := acct["id"].(float64); id != 12345 {
		t.Fatalf("account.id = %v, want 12345", acct["id"])
	}
}

func TestSubscriberListPaginationAndFilters(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v4/subscribers": {200, `{"subscribers":[{"id":1,"email_address":"a@x.com"}],"pagination":{"has_next_page":true,"end_cursor":"CUR","per_page":10}}`},
	})
	defer srv.Close()

	out, errStr, res := run(t, srv.URL+"/v4", tokenEnv(),
		"subscriber", "list", "--status", "all", "--after", "PREV", "--limit", "10")
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, errStr)
	}
	req := findReq(reqs, "GET", "/v4/subscribers")
	if req == nil {
		t.Fatal("GET /v4/subscribers not called")
	}
	if got := req.Query.Get("status"); got != "all" {
		t.Errorf("status = %q, want all", got)
	}
	if got := req.Query.Get("after"); got != "PREV" {
		t.Errorf("after = %q, want PREV", got)
	}
	if got := req.Query.Get("per_page"); got != "10" {
		t.Errorf("per_page = %q, want 10", got)
	}
	env := decode(t, []byte(out))
	if _, ok := env["data"].([]any); !ok {
		t.Fatalf("data is not an array: %s", out)
	}
	pag, ok := env["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("pagination missing from envelope: %s", out)
	}
	if pag["end_cursor"] != "CUR" {
		t.Errorf("pagination.end_cursor = %v, want CUR", pag["end_cursor"])
	}
}

func TestSubscriberListDoesNotAutoPaginate(t *testing.T) {
	// Without an explicit page walk, list must issue exactly one request even
	// though the first page reports has_next_page:true (no unbounded fan-out).
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v4/subscribers": {200, `{"subscribers":[{"id":1}],"pagination":{"has_next_page":true,"end_cursor":"CUR"}}`},
	})
	defer srv.Close()

	_, errStr, res := run(t, srv.URL+"/v4", tokenEnv(), "subscriber", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, errStr)
	}
	n := 0
	for _, r := range reqs {
		if r.Path == "/v4/subscribers" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("subscribers requested %d times, want 1", n)
	}
}

func TestBroadcastCreatePostsBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v4/broadcasts": {201, `{"broadcast":{"id":7,"status":"draft","subject":"Hi"}}`},
	})
	defer srv.Close()

	out, errStr, res := run(t, srv.URL+"/v4", tokenEnv(),
		"broadcast", "create", "--subject", "Hi", "--content", "<p>hi</p>")
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, errStr)
	}
	req := findReq(reqs, "POST", "/v4/broadcasts")
	if req == nil {
		t.Fatal("POST /v4/broadcasts not called")
	}
	if !strings.HasPrefix(req.ContentType, "application/json") {
		t.Errorf("content-type = %q", req.ContentType)
	}
	if req.Auth != "Bearer T" {
		t.Errorf("auth = %q", req.Auth)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if body["subject"] != "Hi" {
		t.Errorf("body.subject = %v, want Hi", body["subject"])
	}
	if body["content"] != "<p>hi</p>" {
		t.Errorf("body.content = %v", body["content"])
	}
	env := decode(t, []byte(out))
	data, ok := env["data"].(map[string]any)
	if !ok || data["id"] == nil {
		t.Fatalf("envelope data missing broadcast: %s", out)
	}
}

func TestTagAddRequiresSubscriberXorEmail(t *testing.T) {
	srv := newMux(t, &[]capturedRequest{}, map[string]stub{})
	defer srv.Close()

	// Neither identifier → usage error, exit 2.
	_, _, res := run(t, srv.URL+"/v4", tokenEnv(), "tag", "add", "--tag-id", "5")
	if res.ExitCode != 2 {
		t.Fatalf("neither id/email: exit=%d, want 2", res.ExitCode)
	}
	// Both identifiers → usage error, exit 2.
	_, _, res = run(t, srv.URL+"/v4", tokenEnv(),
		"tag", "add", "--tag-id", "5", "--subscriber-id", "1", "--email", "a@x.com")
	if res.ExitCode != 2 {
		t.Fatalf("both id+email: exit=%d, want 2", res.ExitCode)
	}
}

func TestTagAddByEmail(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v4/tags/5/subscribers": {200, `{"subscriber":{"id":1}}`},
	})
	defer srv.Close()

	_, errStr, res := run(t, srv.URL+"/v4", tokenEnv(),
		"tag", "add", "--tag-id", "5", "--email", "a@x.com")
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, errStr)
	}
	req := findReq(reqs, "POST", "/v4/tags/5/subscribers")
	if req == nil {
		t.Fatal("POST /v4/tags/5/subscribers not called")
	}
	var body map[string]any
	_ = json.Unmarshal(req.Body, &body)
	if body["email_address"] != "a@x.com" {
		t.Errorf("body.email_address = %v, want a@x.com", body["email_address"])
	}
}

func TestAPIErrorPlainAndJSON(t *testing.T) {
	routes := map[string]stub{
		"GET /v4/account": {422, `{"errors":["Something went wrong"]}`},
	}
	// Plain error rendering.
	var reqs []capturedRequest
	srv := newMux(t, &reqs, routes)
	defer srv.Close()
	_, errStr, res := run(t, srv.URL+"/v4", tokenEnv(), "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", res.ExitCode)
	}
	if !strings.Contains(errStr, "Something went wrong") {
		t.Errorf("plain error missing message: %q", errStr)
	}
	if strings.TrimSpace(errStr) == "" || strings.HasPrefix(strings.TrimSpace(errStr), "{") {
		t.Errorf("plain mode should not emit JSON: %q", errStr)
	}

	// JSON error envelope.
	var reqs2 []capturedRequest
	srv2 := newMux(t, &reqs2, routes)
	defer srv2.Close()
	_, errStr2, res2 := run(t, srv2.URL+"/v4", tokenEnv(), "account", "get", "--json")
	if res2.ExitCode != 1 {
		t.Fatalf("json exit=%d, want 1", res2.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errStr2), &env); err != nil {
		t.Fatalf("json error not decodable: %v (%q)", err, errStr2)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("json error kind/status = %q/%d", env.Error.Kind, env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "Something went wrong") {
		t.Errorf("json error message = %q", env.Error.Message)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	srv := newMux(t, &[]capturedRequest{}, map[string]stub{})
	defer srv.Close()
	_, errStr, res := run(t, srv.URL+"/v4", map[string]string{}, "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", res.ExitCode)
	}
	if !strings.Contains(errStr, EnvToken) {
		t.Errorf("error should name %s: %q", EnvToken, errStr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v4/account": {401, `{"errors":["unauthorized"]}`},
	})
	defer srv.Close()
	_, _, res := run(t, srv.URL+"/v4", tokenEnv(), "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("401 should mark CredentialRejected")
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	srv := newMux(t, &[]capturedRequest{}, map[string]stub{})
	defer srv.Close()
	_, _, res := run(t, srv.URL+"/v4", tokenEnv(), "subscriber", "frobnicate")
	if res.ExitCode != 2 {
		t.Fatalf("exit=%d, want 2", res.ExitCode)
	}
}
