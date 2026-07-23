package omnisend

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// run executes one omnisend invocation against the given fake server, returning
// the result, stdout, and stderr.
func run(t *testing.T, srv string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	s := &Service{BaseURL: srv, Out: &out, Err: &errOut}
	res, err := s.Execute(context.Background(), args, map[string]string{EnvAccessToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res, out.String(), errOut.String()
}

func TestContactList(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {200, `{"contacts":[{"contactID":"c1"}],"paging":{"cursors":{"after":"CUR2"},"hasMore":true}}`},
	})
	defer mux.Close()

	res, out, errOut := run(t, mux.URL, "contact", "list", "--limit", "25", "--email", "a@b.com")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/contacts")
	if req == nil {
		t.Fatal("no GET /contacts recorded")
	}
	if req.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", req.Auth)
	}
	if req.Version != "2026-03-15" {
		t.Errorf("Omnisend-Version = %q, want 2026-03-15", req.Version)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
	if got := req.Query.Get("limit"); got != "25" {
		t.Errorf("limit = %q, want 25", got)
	}
	if got := req.Query.Get("email"); got != "a@b.com" {
		t.Errorf("email = %q, want a@b.com", got)
	}
	if !strings.Contains(out, `"contactID":"c1"`) {
		t.Errorf("stdout did not pass provider JSON through: %q", out)
	}
}

func TestContactListPaginationAfter(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {200, `{"contacts":[]}`},
	})
	defer mux.Close()

	run(t, mux.URL, "contact", "list", "--after", "CUR2")
	req := findReq(reqs, "GET", "/contacts")
	if req == nil {
		t.Fatal("no GET /contacts recorded")
	}
	if got := req.Query.Get("after"); got != "CUR2" {
		t.Errorf("after = %q, want CUR2", got)
	}
	if _, present := req.Query["limit"]; present {
		t.Error("limit should be omitted when 0")
	}
}

func TestContactGet(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /contacts/c9": {200, `{"contactID":"c9"}`},
	})
	defer mux.Close()

	res, out, errOut := run(t, mux.URL, "contact", "get", "--id", "c9")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/contacts/c9") == nil {
		t.Fatal("no GET /contacts/c9 recorded")
	}
	if !strings.Contains(out, `"contactID":"c9"`) {
		t.Errorf("stdout = %q", out)
	}
}

func TestContactCreatePassesRawBody(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"POST /contacts": {200, `{"contactID":"new1"}`},
	})
	defer mux.Close()

	res, _, errOut := run(t, mux.URL, "contact", "create", "--data",
		`{"identifiers":[{"type":"email","id":"x@y.com"}],"firstName":"X"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/contacts")
	if req == nil {
		t.Fatal("no POST /contacts recorded")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["firstName"] != "X" {
		t.Errorf("body firstName = %v, want X", body["firstName"])
	}
	if _, ok := body["identifiers"]; !ok {
		t.Errorf("body missing identifiers: %s", req.Body)
	}
}

func TestContactUpdate(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"PATCH /contacts/c5": {200, `{"contactID":"c5"}`},
	})
	defer mux.Close()

	res, _, errOut := run(t, mux.URL, "contact", "update", "--id", "c5", "--data", `{"firstName":"Z"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "PATCH", "/contacts/c5")
	if req == nil {
		t.Fatal("no PATCH /contacts/c5 recorded")
	}
	if bodyMap(t, req.Body)["firstName"] != "Z" {
		t.Errorf("body = %s", req.Body)
	}
}

func TestEventSend(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"POST /events": {202, `{"eventID":"e1"}`},
	})
	defer mux.Close()

	res, _, errOut := run(t, mux.URL, "event", "send", "--data",
		`{"eventName":"trial started","contact":{"email":"x@y.com"}}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/events")
	if req == nil {
		t.Fatal("no POST /events recorded")
	}
	if bodyMap(t, req.Body)["eventName"] != "trial started" {
		t.Errorf("body = %s", req.Body)
	}
}

func TestCampaignAndSegmentAndProductPaths(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /campaigns":      {200, `{"campaigns":[]}`},
		"GET /campaigns/ca1":  {200, `{"campaignID":"ca1"}`},
		"GET /segments":       {200, `{"segments":[]}`},
		"GET /segments/sg1":   {200, `{"segmentID":"sg1"}`},
		"POST /segments":      {200, `{"segmentID":"new"}`},
		"GET /products":       {200, `{"products":[]}`},
		"GET /products/p1":    {200, `{"productID":"p1"}`},
		"GET /brands/current": {200, `{"brandID":"b1","website":"shop.example.com"}`},
	})
	defer mux.Close()

	cases := []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"campaign", "list"}, "GET", "/campaigns"},
		{[]string{"campaign", "get", "--id", "ca1"}, "GET", "/campaigns/ca1"},
		{[]string{"segment", "list"}, "GET", "/segments"},
		{[]string{"segment", "get", "--id", "sg1"}, "GET", "/segments/sg1"},
		{[]string{"segment", "create", "--data", `{"name":"VIP"}`}, "POST", "/segments"},
		{[]string{"product", "list"}, "GET", "/products"},
		{[]string{"product", "get", "--id", "p1"}, "GET", "/products/p1"},
		{[]string{"brand", "get"}, "GET", "/brands/current"},
	}
	for _, c := range cases {
		res, _, errOut := run(t, mux.URL, c.args...)
		if res.ExitCode != 0 {
			t.Fatalf("%v: exit = %d, stderr = %q", c.args, res.ExitCode, errOut)
		}
		if findReq(reqs, c.method, c.path) == nil {
			t.Errorf("%v: no %s %s recorded", c.args, c.method, c.path)
		}
	}
}

func TestBatchGetAndCreate(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /batches/bt1": {200, `{"batchID":"bt1","status":"pending"}`},
		"POST /batches":    {200, `{"batchID":"bt2"}`},
	})
	defer mux.Close()

	if res, _, _ := run(t, mux.URL, "batch", "get", "--id", "bt1"); res.ExitCode != 0 {
		t.Fatalf("batch get exit = %d", res.ExitCode)
	}
	if findReq(reqs, "GET", "/batches/bt1") == nil {
		t.Error("no GET /batches/bt1 recorded")
	}
	if res, _, _ := run(t, mux.URL, "batch", "create", "--data", `{"method":"POST","endpoint":"contacts","items":[]}`); res.ExitCode != 0 {
		t.Fatalf("batch create exit = %d", res.ExitCode)
	}
	if findReq(reqs, "POST", "/batches") == nil {
		t.Error("no POST /batches recorded")
	}
}

func TestMissingTokenExit1(t *testing.T) {
	var out, errOut bytes.Buffer
	s := &Service{Out: &out, Err: &errOut}
	res, err := s.Execute(context.Background(), []string{"contact", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "OMNISEND_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestAPIErrorExit1PlainAndJSON(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {500, `{"error":{"message":"boom"}}`},
	})
	defer mux.Close()

	res, out, errOut := run(t, mux.URL, "contact", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("500 must not reject the credential")
	}
	if out != "" {
		t.Errorf("stdout should be empty on error, got %q", out)
	}
	if !strings.Contains(errOut, "boom") || !strings.Contains(errOut, "HTTP 500") {
		t.Errorf("plain stderr = %q", errOut)
	}

	// --json error envelope
	res2, _, errOut2 := run(t, mux.URL, "--json", "contact", "list")
	if res2.ExitCode != 1 {
		t.Fatalf("json exit = %d, want 1", res2.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut2)), &env); err != nil {
		t.Fatalf("json stderr not an envelope: %v (%q)", err, errOut2)
	}
	if env.Error.Kind != "api" || env.Error.Status != 500 {
		t.Errorf("envelope = %+v, want kind=api status=500", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {401, `{"error":{"message":"invalid token"}}`},
	})
	defer mux.Close()

	res, _, _ := run(t, mux.URL, "contact", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
}

func TestUsageErrorExit2(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{})
	defer mux.Close()

	// missing required --id
	if res, _, _ := run(t, mux.URL, "contact", "get"); res.ExitCode != 2 {
		t.Errorf("missing --id exit = %d, want 2", res.ExitCode)
	}
	// invalid JSON to --data
	if res, _, _ := run(t, mux.URL, "contact", "create", "--data", "{not json"); res.ExitCode != 2 {
		t.Errorf("bad json exit = %d, want 2", res.ExitCode)
	}
	// unknown subcommand
	if res, _, _ := run(t, mux.URL, "contact", "bogus"); res.ExitCode != 2 {
		t.Errorf("unknown subcommand exit = %d, want 2", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("usage errors should not hit the API, saw %d requests", len(reqs))
	}
}

func TestVersionHeaderOnEveryCall(t *testing.T) {
	var reqs []capturedRequest
	mux := newMux(t, &reqs, map[string]stub{
		"GET /brands/current": {200, `{"brandID":"b1"}`},
		"POST /events":        {202, `{}`},
	})
	defer mux.Close()

	run(t, mux.URL, "brand", "get")
	run(t, mux.URL, "event", "send", "--data", `{"eventName":"x"}`)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	for _, r := range reqs {
		if r.Version != "2026-03-15" {
			t.Errorf("%s %s: version = %q, want 2026-03-15", r.Method, r.Path, r.Version)
		}
	}
}
