package later

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const tokenBody = `{"jwt":"jwt-abc"}`

func TestSplitCredentials(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantID     string
		wantSecret string
		wantOK     bool
	}{
		{"simple", "cid:secret", "cid", "secret", true},
		{"secret-with-colon", "cid:sec:ret:part", "cid", "sec:ret:part", true},
		{"trimmed", "  cid : secret  ", "cid", "secret", true},
		{"missing-colon", "cidsecret", "", "", false},
		{"empty-id", ":secret", "", "", false},
		{"empty-secret", "cid:", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, secret, ok := splitCredentials(c.raw)
			if ok != c.wantOK || id != c.wantID || secret != c.wantSecret {
				t.Fatalf("splitCredentials(%q) = (%q,%q,%v), want (%q,%q,%v)",
					c.raw, id, secret, ok, c.wantID, c.wantSecret, c.wantOK)
			}
		})
	}
}

func TestExecuteMissingCredentialsFailsFast(t *testing.T) {
	srv, _ := newFakeServer(t, map[string]routeReply{})
	result, _, stderr := runResult(t, srv, "not-a-pair", "instances")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("a locally malformed credential must not be classified as provider-rejected")
	}
	if !strings.Contains(stderr, "LATER_CREDENTIALS") {
		t.Fatalf("stderr = %q, want a LATER_CREDENTIALS hint", stderr)
	}
}

func TestInstancesMintsThenCalls(t *testing.T) {
	instancesBody := `{"data":{"instanceIds":["instance_abc","instance_def"]},"nextCursor":null}`
	srv, fake := newFakeServer(t, map[string]routeReply{
		"/oauth/token":  {status: 200, body: tokenBody},
		"/v2/instances": {status: 200, body: instancesBody},
	})

	exit, stdout, stderr := run(t, srv, "instances", "--limit", "50")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if strings.TrimSpace(stdout) != instancesBody {
		t.Fatalf("stdout = %q, want provider body verbatim", stdout)
	}

	// The token exchange POSTed the client-credentials pair as JSON.
	tokenReq, tokenHits := fake.get("/oauth/token")
	if tokenHits != 1 || tokenReq.Method != "POST" {
		t.Fatalf("token route: hits=%d method=%s, want 1 POST", tokenHits, tokenReq.Method)
	}
	var body tokenRequest
	if err := json.Unmarshal(tokenReq.Body, &body); err != nil {
		t.Fatalf("token body not JSON: %v (%s)", err, tokenReq.Body)
	}
	if body.ClientID != "cid-123" || body.ClientSecret != "secret-xyz" {
		t.Fatalf("token body = %+v, want cid-123/secret-xyz", body)
	}

	// The data call carried the minted JWT and the limit query.
	dataReq, dataHits := fake.get("/v2/instances")
	if dataHits != 1 {
		t.Fatalf("instances hits = %d, want 1", dataHits)
	}
	if dataReq.Auth != "Bearer jwt-abc" {
		t.Fatalf("instances auth = %q, want Bearer jwt-abc", dataReq.Auth)
	}
	if q := parseQuery(t, dataReq.Query); q.Get("limit") != "50" {
		t.Fatalf("instances limit = %q, want 50", q.Get("limit"))
	}
}

func TestInstancesOmitsLimitWhenUnset(t *testing.T) {
	srv, fake := newFakeServer(t, map[string]routeReply{
		"/oauth/token":  {status: 200, body: tokenBody},
		"/v2/instances": {status: 200, body: `{"data":{"instanceIds":[]},"nextCursor":null}`},
	})
	if exit, _, stderr := run(t, srv, "instances"); exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	dataReq, _ := fake.get("/v2/instances")
	if parseQuery(t, dataReq.Query).Has("limit") {
		t.Fatalf("instances query = %q, want no limit param", dataReq.Query)
	}
}

func TestCampaignsEncodesFilters(t *testing.T) {
	srv, fake := newFakeServer(t, map[string]routeReply{
		"/oauth/token":              {status: 200, body: tokenBody},
		"/v2/campaigns/performance": {status: 200, body: `{"data":[]}`},
	})

	exit, _, stderr := run(t, srv,
		"campaigns",
		"--start", "2025-01-01",
		"--end", "2025-01-31",
		"--metrics", "engagements,impressions",
		"--instance-ids", "instance_abc,instance_def",
		"--platform", "instagram",
		"--sort", "estimatedRoi",
		"--sort-dir", "DESC",
		"--limit", "25",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	dataReq, _ := fake.get("/v2/campaigns/performance")
	q := parseQuery(t, dataReq.Query)
	if q.Get("startDate") != "2025-01-01" || q.Get("endDate") != "2025-01-31" {
		t.Fatalf("date range = %q..%q", q.Get("startDate"), q.Get("endDate"))
	}
	if got := q["metrics"]; len(got) != 2 || got[0] != "engagements" || got[1] != "impressions" {
		t.Fatalf("metrics = %v, want [engagements impressions] as repeated params", got)
	}
	if got := q["instanceIds"]; len(got) != 2 {
		t.Fatalf("instanceIds = %v, want two repeated params", got)
	}
	if q.Get("platform") != "instagram" || q.Get("sortProperty") != "estimatedRoi" || q.Get("sortDirection") != "DESC" || q.Get("limit") != "25" {
		t.Fatalf("filters not encoded: %q", dataReq.Query)
	}
}

func TestCampaignsRequiresDates(t *testing.T) {
	srv, _ := newFakeServer(t, map[string]routeReply{
		"/oauth/token": {status: 200, body: tokenBody},
	})
	// Missing --end: cobra required-flag error, non-zero exit, no data call.
	exit, _, stderr := run(t, srv, "campaigns", "--start", "2025-01-01")
	if exit == 0 {
		t.Fatal("expected non-zero exit when --end is missing")
	}
	if !strings.Contains(stderr, "end") {
		t.Fatalf("stderr = %q, want a required-flag error mentioning end", stderr)
	}
}

func TestDataCall401ReMintsOnceCounting(t *testing.T) {
	instancesBody := `{"data":{"instanceIds":["instance_abc"]},"nextCursor":null}`
	srv, fake := newCountingServer(t, instancesBody)
	exit, stdout, stderr := run(t, srv, "instances")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if strings.TrimSpace(stdout) != instancesBody {
		t.Fatalf("stdout = %q, want body after re-mint", stdout)
	}
	if fake.tokenHits != 2 {
		t.Fatalf("token mints = %d, want 2 (initial + one re-mint on 401)", fake.tokenHits)
	}
	if fake.dataHits != 2 {
		t.Fatalf("data hits = %d, want 2 (401 then retry)", fake.dataHits)
	}
}

func TestBadCredentialPairIsRejected(t *testing.T) {
	srv, _ := newFakeServer(t, map[string]routeReply{
		"/oauth/token": {status: 401, body: `{"error":"invalid client"}`},
	})
	result, _, stderr := runResult(t, srv, testCredentials, "instances")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatalf("a 401 from /oauth/token must reject the credential; stderr=%q", stderr)
	}
}

func TestPersistent401AfterReMintIsRejected(t *testing.T) {
	srv, _ := newFakeServer(t, map[string]routeReply{
		"/oauth/token":  {status: 200, body: tokenBody},
		"/v2/instances": {status: 401, body: `{"error":"forbidden token"}`},
	})
	result, _, _ := runResult(t, srv, testCredentials, "instances")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("persistent 401 must reject: result=%+v", result)
	}
}

func TestServerErrorIsNotCredentialRejection(t *testing.T) {
	srv, _ := newFakeServer(t, map[string]routeReply{
		"/oauth/token":  {status: 200, body: tokenBody},
		"/v2/instances": {status: 500, body: `{"error":"boom"}`},
	})
	result, _, _ := runResult(t, srv, testCredentials, "instances")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("a 500 must not invalidate the credential")
	}
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}
