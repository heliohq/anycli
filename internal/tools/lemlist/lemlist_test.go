package lemlist

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const testKey = "lemlist_test_key_123"

// wantBasicAuth is the exact Authorization header value Lemlist requires: Basic
// auth with an EMPTY username and the API key as the password —
// base64(":"+key).
func wantBasicAuth(key string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+key))
}

// run executes the lemlist service against a fake server and returns exit code,
// stdout, and stderr.
func run(t *testing.T, baseURL string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: baseURL, Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: testKey})
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	return res.ExitCode, out.String(), errBuf.String()
}

// TestBasicAuthEmptyUser is the load-bearing auth assertion: every request must
// carry Basic auth with an empty username and the key as the password.
func TestBasicAuthEmptyUser(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /team": {status: 200, body: `{"_id":"tea_1","name":"lemlist"}`},
	})
	defer srv.Close()

	code, out, errStr := run(t, srv.URL, "team", "get")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/team")
	if req == nil {
		t.Fatal("no GET /team request recorded")
	}
	if req.Auth != wantBasicAuth(testKey) {
		t.Errorf("Authorization = %q, want %q", req.Auth, wantBasicAuth(testKey))
	}
	// Decode the Basic credentials and confirm the username is empty.
	raw := strings.TrimPrefix(req.Auth, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode basic auth: %v", err)
	}
	user, pass, found := strings.Cut(string(decoded), ":")
	if !found || user != "" || pass != testKey {
		t.Errorf("decoded basic = %q (user=%q pass=%q), want empty user + key", decoded, user, pass)
	}
	if strings.TrimSpace(out) != `{"_id":"tea_1","name":"lemlist"}` {
		t.Errorf("stdout = %q, want passthrough JSON", out)
	}
}

func TestTeamAndCampaignRoutes(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  map[string]string
	}{
		{"team senders", []string{"team", "senders"}, "GET", "/team/senders", nil},
		{"team credits", []string{"team", "credits"}, "GET", "/team/credits", nil},
		{"campaign list", []string{"campaign", "list", "--status", "running", "--limit", "50"}, "GET", "/campaigns", map[string]string{"version": "v2", "status": "running", "limit": "50"}},
		{"campaign get", []string{"campaign", "get", "cam_9"}, "GET", "/campaigns/cam_9", nil},
		{"campaign stats", []string{"campaign", "stats", "cam_9", "--start-date", "2024-01-01", "--end-date", "2024-01-31"}, "GET", "/v2/campaigns/cam_9/stats", map[string]string{"startDate": "2024-01-01", "endDate": "2024-01-31"}},
		{"campaign start", []string{"campaign", "start", "cam_9"}, "POST", "/campaigns/cam_9/start", nil},
		{"campaign pause", []string{"campaign", "pause", "cam_9"}, "POST", "/campaigns/cam_9/pause", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				tc.wantMethod + " " + tc.wantPath: {status: 200, body: `{"ok":true}`},
			})
			defer srv.Close()

			code, _, errStr := run(t, srv.URL, tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, stderr=%s", code, errStr)
			}
			req := findReq(reqs, tc.wantMethod, tc.wantPath)
			if req == nil {
				t.Fatalf("no %s %s request recorded; got %+v", tc.wantMethod, tc.wantPath, reqs)
			}
			for k, want := range tc.wantQuery {
				if got := req.Query.Get(k); got != want {
					t.Errorf("query[%s] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestLeadAddBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /campaigns/cam_1/leads/": {status: 200, body: `{"_id":"lea_1"}`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL,
		"lead", "add", "cam_1",
		"--email", "jane@acme.com",
		"--first-name", "Jane",
		"--fields", `{"customVar":"x"}`,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "POST", "/campaigns/cam_1/leads/")
	if req == nil {
		t.Fatal("no POST /campaigns/cam_1/leads/ recorded")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["email"] != "jane@acme.com" {
		t.Errorf("email = %v, want jane@acme.com", body["email"])
	}
	if body["firstName"] != "Jane" {
		t.Errorf("firstName = %v, want Jane", body["firstName"])
	}
	if body["customVar"] != "x" {
		t.Errorf("customVar = %v, want x (merged from --fields)", body["customVar"])
	}
}

func TestLeadGetRequiresEmailOrID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL, "lead", "get")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr=%s", code, errStr)
	}
	if len(reqs) != 0 {
		t.Errorf("expected no HTTP call for a usage error, got %d", len(reqs))
	}
}

func TestLeadGetQueryAndUpdateAndDisposition(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"lead get by email", []string{"lead", "get", "--email", "jane@acme.com"}, "GET", "/leads"},
		{"lead update", []string{"lead", "update", "cam_1", "lea_1", "--fields", `{"firstName":"J"}`}, "PATCH", "/campaigns/cam_1/leads/lea_1"},
		{"lead unsubscribe", []string{"lead", "unsubscribe", "cam_1", "jane@acme.com"}, "DELETE", "/campaigns/cam_1/leads/jane@acme.com"},
		{"lead delete", []string{"lead", "delete", "cam_1", "lea_1"}, "DELETE", "/campaigns/cam_1/leads/lea_1"},
		{"mark interested", []string{"lead", "mark-interested", "lea_1"}, "POST", "/leads/interested/lea_1"},
		{"mark not interested", []string{"lead", "mark-not-interested", "lea_1"}, "POST", "/leads/notinterested/lea_1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				tc.wantMethod + " " + tc.wantPath: {status: 200, body: `{"ok":true}`},
			})
			defer srv.Close()

			code, _, errStr := run(t, srv.URL, tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, stderr=%s", code, errStr)
			}
			if findReq(reqs, tc.wantMethod, tc.wantPath) == nil {
				t.Fatalf("no %s %s recorded; got %+v", tc.wantMethod, tc.wantPath, reqs)
			}
		})
	}
}

// TestLeadDeleteForcesActionRemove is the load-bearing delete assertion:
// Lemlist's DELETE /campaigns/{id}/leads/{leadId} only force-deletes when
// action=remove is sent; without it the endpoint silently falls back to
// unsubscribing (and expects an email, not a lead id). The `delete` verb must
// therefore always send action=remove.
func TestLeadDeleteForcesActionRemove(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"DELETE /campaigns/cam_1/leads/lea_1": {status: 200, body: `{"ok":true}`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL, "lead", "delete", "cam_1", "lea_1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "DELETE", "/campaigns/cam_1/leads/lea_1")
	if req == nil {
		t.Fatal("no DELETE /campaigns/cam_1/leads/lea_1 recorded")
	}
	if got := req.Query.Get("action"); got != "remove" {
		t.Errorf("action = %q, want remove (force delete)", got)
	}
}

// TestCampaignStatsRequiresDates guards the required window: Lemlist documents
// both startDate and endDate as required for GET /v2/campaigns/{id}/stats, so a
// bare `campaign stats <id>` must fail with a usage error (exit 2) rather than
// issue a request that the API rejects with HTTP 400.
func TestCampaignStatsRequiresDates(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL, "campaign", "stats", "cam_9")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage) for missing dates; stderr=%s", code, errStr)
	}
	if len(reqs) != 0 {
		t.Errorf("expected no HTTP call for a usage error, got %d", len(reqs))
	}
}

func TestActivityListAlwaysSendsVersionV2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /activities": {status: 200, body: `[]`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL, "activity", "list", "--type", "emailsOpened")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/activities")
	if req == nil {
		t.Fatal("no GET /activities recorded")
	}
	if req.Query.Get("version") != "v2" {
		t.Errorf("version = %q, want v2 (mandatory)", req.Query.Get("version"))
	}
	if req.Query.Get("type") != "emailsOpened" {
		t.Errorf("type = %q, want emailsOpened", req.Query.Get("type"))
	}
}

func TestUnsubscribeRoutes(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"list", []string{"unsubscribe", "list"}, "GET", "/unsubscribes"},
		{"add", []string{"unsubscribe", "add", "jane@acme.com"}, "POST", "/unsubscribes/jane@acme.com"},
		{"delete", []string{"unsubscribe", "delete", "acme.com"}, "DELETE", "/unsubscribes/acme.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				tc.wantMethod + " " + tc.wantPath: {status: 200, body: `{"ok":true}`},
			})
			defer srv.Close()

			code, _, errStr := run(t, srv.URL, tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, stderr=%s", code, errStr)
			}
			if findReq(reqs, tc.wantMethod, tc.wantPath) == nil {
				t.Fatalf("no %s %s recorded; got %+v", tc.wantMethod, tc.wantPath, reqs)
			}
		})
	}
}

func TestAPIErrorExitOneWithJSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /team": {status: 500, body: `{"message":"boom"}`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv.URL, "team", "get", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, errStr)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errStr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errStr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 500 {
		t.Errorf("envelope = %+v, want kind=api status=500", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /team": {status: 401, body: `{"message":"unauthorized"}`},
	})
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"team", "get"}, map[string]string{EnvAPIKey: testKey})
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("CredentialRejected = false, want true on HTTP 401")
	}
}

func TestMissingKeyExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"team", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1 for missing key", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey) {
		t.Errorf("stderr = %q, want mention of %s", errBuf.String(), EnvAPIKey)
	}
}

func TestUnknownSubcommandExitsTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, _ := run(t, srv.URL, "team", "bogus")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", code)
	}
}
