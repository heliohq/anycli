package braze

import (
	"reflect"
	"testing"
)

// TestGETQueryParamMapping — GET verbs map their flags to the exact Braze query
// parameters (campaign_id, length, ending_at, …).
func TestGETQueryParamMapping(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /campaigns/data_series": {status: 200, body: `{"data":[]}`},
		"GET /kpi/dau/data_series":   {status: 200, body: `{"data":[]}`},
		"GET /events/data_series":    {status: 200, body: `{"data":[]}`},
	})
	defer srv.Close()

	if exit, _, stderr := run(t, srv, "campaigns", "series", "--campaign-id", "c1", "--length", "30", "--ending-at", "2026-07-01"); exit != 0 {
		t.Fatalf("campaigns series exit=%d stderr=%s", exit, stderr)
	}
	req := findReq(reqs, "GET", "/campaigns/data_series")
	if req == nil {
		t.Fatal("no campaigns/data_series request")
	}
	if req.Query.Get("campaign_id") != "c1" || req.Query.Get("length") != "30" || req.Query.Get("ending_at") != "2026-07-01" {
		t.Fatalf("query = %v, want campaign_id=c1 length=30 ending_at=2026-07-01", req.Query)
	}

	if exit, _, _ := run(t, srv, "kpi", "dau", "--length", "7"); exit != 0 {
		t.Fatalf("kpi dau exit=%d", exit)
	}
	if req := findReq(reqs, "GET", "/kpi/dau/data_series"); req == nil || req.Query.Get("length") != "7" {
		t.Fatalf("kpi dau query = %v", req)
	}

	if exit, _, _ := run(t, srv, "events", "series", "--event", "purchase", "--unit", "hour", "--length", "24"); exit != 0 {
		t.Fatalf("events series exit=%d", exit)
	}
	req = findReq(reqs, "GET", "/events/data_series")
	if req == nil || req.Query.Get("event") != "purchase" || req.Query.Get("unit") != "hour" {
		t.Fatalf("events series query = %v", req)
	}
}

// TestUsersExportIsPOSTWithIdentifierBody — users export uses POST (not GET) and
// assembles the identifier body; a call with no identifier is a usage error.
func TestUsersExportIsPOSTWithIdentifierBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /users/export/ids": {status: 200, body: `{"users":[]}`},
	})
	defer srv.Close()

	exit, _, stderr := run(t, srv, "users", "export", "--external-id", "u1", "--external-id", "u2", "--fields", "email", "--fields", "custom_attributes")
	if exit != 0 {
		t.Fatalf("users export exit=%d stderr=%s", exit, stderr)
	}
	req := findReq(reqs, "POST", "/users/export/ids")
	if req == nil {
		t.Fatal("users export did not POST /users/export/ids")
	}
	if req.ContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", req.ContentType)
	}
	body := decodeBody(t, req.Body)
	ids, _ := body["external_ids"].([]any)
	if len(ids) != 2 || ids[0] != "u1" {
		t.Fatalf("external_ids = %v, want [u1 u2]", body["external_ids"])
	}
	fields, _ := body["fields_to_export"].([]any)
	if !reflect.DeepEqual(fields, []any{"email", "custom_attributes"}) {
		t.Fatalf("fields_to_export = %v", body["fields_to_export"])
	}

	// --email is a single string per the Braze contract ("only one email_address
	// can be included per request"), not a JSON array.
	if exit, _, stderr := run(t, srv, "users", "export", "--email", "a@b.co"); exit != 0 {
		t.Fatalf("users export --email exit=%d stderr=%s", exit, stderr)
	}
	emailReq := findReq(reqs[len(reqs)-1:], "POST", "/users/export/ids")
	if emailReq == nil {
		t.Fatal("users export --email did not POST /users/export/ids")
	}
	emailBody := decodeBody(t, emailReq.Body)
	if got, ok := emailBody["email_address"].(string); !ok || got != "a@b.co" {
		t.Fatalf("email_address = %#v, want string \"a@b.co\"", emailBody["email_address"])
	}

	// --braze-id is likewise a single string, not an array.
	if exit, _, stderr := run(t, srv, "users", "export", "--braze-id", "bz1"); exit != 0 {
		t.Fatalf("users export --braze-id exit=%d stderr=%s", exit, stderr)
	}
	brazeReq := findReq(reqs[len(reqs)-1:], "POST", "/users/export/ids")
	if brazeReq == nil {
		t.Fatal("users export --braze-id did not POST /users/export/ids")
	}
	brazeBody := decodeBody(t, brazeReq.Body)
	if got, ok := brazeBody["braze_id"].(string); !ok || got != "bz1" {
		t.Fatalf("braze_id = %#v, want string \"bz1\"", brazeBody["braze_id"])
	}

	// No identifier → usage error, exit 2, no request.
	before := len(reqs)
	result, _, _ := runResult(t, srv, "users", "export", "--fields", "email")
	if result.ExitCode != 2 {
		t.Fatalf("no-identifier exit = %d, want 2", result.ExitCode)
	}
	if len(reqs) != before {
		t.Fatal("no-identifier export still hit the API")
	}
}

// TestPOSTRawBodyPassthrough — messages send passes the --body object through
// verbatim; canvas trigger overlays canvas_id onto the passthrough body.
func TestPOSTRawBodyPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /messages/send":       {status: 201, body: `{"message":"success","dispatch_id":"d1"}`},
		"POST /canvas/trigger/send": {status: 201, body: `{"message":"success"}`},
		"POST /users/track":         {status: 201, body: `{"message":"success"}`},
	})
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "messages", "send", "--body", `{"broadcast":true,"messages":{"apple_push":{"alert":"hi"}}}`)
	if exit != 0 {
		t.Fatalf("messages send exit=%d stderr=%s", exit, stderr)
	}
	req := findReq(reqs, "POST", "/messages/send")
	body := decodeBody(t, req.Body)
	if body["broadcast"] != true {
		t.Fatalf("messages send body lost broadcast: %v", body)
	}
	if _, ok := body["messages"].(map[string]any); !ok {
		t.Fatalf("messages send body lost messages object: %v", body)
	}
	if stdout != `{"message":"success","dispatch_id":"d1"}`+"\n" {
		t.Fatalf("stdout = %q, want verbatim passthrough", stdout)
	}

	if exit, _, _ := run(t, srv, "canvas", "trigger", "--canvas-id", "cv1", "--body", `{"broadcast":true}`); exit != 0 {
		t.Fatalf("canvas trigger exit=%d", exit)
	}
	req = findReq(reqs, "POST", "/canvas/trigger/send")
	body = decodeBody(t, req.Body)
	if body["canvas_id"] != "cv1" || body["broadcast"] != true {
		t.Fatalf("canvas trigger body = %v, want canvas_id=cv1 + broadcast", body)
	}

	if exit, _, _ := run(t, srv, "users", "track", "--attributes", `[{"external_id":"u1","first_name":"A"}]`); exit != 0 {
		t.Fatalf("users track exit=%d", exit)
	}
	req = findReq(reqs, "POST", "/users/track")
	body = decodeBody(t, req.Body)
	if _, ok := body["attributes"].([]any); !ok {
		t.Fatalf("users track body lost attributes array: %v", body)
	}
}

// TestSubscriptionStatusSetBody — status-set assembles the group/state/identifier
// body and validates the state enum.
func TestSubscriptionStatusSetBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /subscription/status/set": {status: 201, body: `{"message":"success"}`},
	})
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "status-set", "--subscription-group-id", "g1", "--state", "unsubscribed", "--email", "a@b.co")
	if exit != 0 {
		t.Fatalf("status-set exit=%d stderr=%s", exit, stderr)
	}
	req := findReq(reqs, "POST", "/subscription/status/set")
	body := decodeBody(t, req.Body)
	if body["subscription_group_id"] != "g1" || body["subscription_state"] != "unsubscribed" {
		t.Fatalf("status-set body = %v", body)
	}

	// Bad state enum → usage error, exit 2.
	result, _, _ := runResult(t, srv, "subscription", "status-set", "--subscription-group-id", "g1", "--state", "maybe", "--email", "a@b.co")
	if result.ExitCode != 2 {
		t.Fatalf("bad state exit = %d, want 2", result.ExitCode)
	}
}

// TestMissingRequiredFlagIsUsageExitTwo — a missing required flag never reaches
// the API and exits 2.
func TestMissingRequiredFlagIsUsageExitTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	result, _, _ := runResult(t, srv, "campaigns", "details")
	if result.ExitCode != 2 {
		t.Fatalf("missing --campaign-id exit = %d, want 2", result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Fatal("missing required flag still hit the API")
	}
}
