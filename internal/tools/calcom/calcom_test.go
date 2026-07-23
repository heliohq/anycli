package calcom

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPerRouteVersion is the load-bearing invariant: Cal.com v2 pins its API
// version PER ENDPOINT FAMILY, so each command must send its own cal-api-version
// date. A single fixed value would silently downgrade some endpoints' response
// semantics. This asserts the exact date every command sends.
func TestPerRouteVersion(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantMethod  string
		wantPath    string
		wantVersion string
	}{
		{"event-type list", []string{"event-type", "list"}, "GET", "/event-types", "2024-06-14"},
		{"event-type get", []string{"event-type", "get", "--id", "42"}, "GET", "/event-types/42", "2024-06-14"},
		{"slot list", []string{"slot", "list", "--event-type-id", "42", "--start", "2026-01-01T00:00:00Z", "--end", "2026-01-08T00:00:00Z"}, "GET", "/slots", "2024-09-04"},
		{"booking list", []string{"booking", "list"}, "GET", "/bookings", "2024-08-13"},
		{"booking get", []string{"booking", "get", "--uid", "abc"}, "GET", "/bookings/abc", "2024-08-13"},
		{"booking cancel", []string{"booking", "cancel", "--uid", "abc"}, "POST", "/bookings/abc/cancel", "2024-08-13"},
		{"booking reschedule", []string{"booking", "reschedule", "--uid", "abc", "--start", "2026-01-01T00:00:00Z"}, "POST", "/bookings/abc/reschedule", "2024-08-13"},
		{"schedule list", []string{"schedule", "list"}, "GET", "/schedules", "2024-06-11"},
		{"me", []string{"me"}, "GET", "/me", "2024-06-14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newServer(t, &reqs, map[string]route{
				tc.wantMethod + " " + tc.wantPath: {status: 200, body: `{"status":"success","data":{}}`},
			})
			defer srv.Close()

			exit, _, stderr := run(t, srv, tc.args...)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr = %s", exit, stderr)
			}
			req := findReq(reqs, tc.wantMethod, tc.wantPath)
			if req == nil {
				t.Fatalf("no %s %s request; got %+v", tc.wantMethod, tc.wantPath, reqs)
			}
			if req.Version != tc.wantVersion {
				t.Errorf("cal-api-version = %q, want %q", req.Version, tc.wantVersion)
			}
			if req.Auth != "Bearer tok-123" {
				t.Errorf("Authorization = %q, want Bearer tok-123", req.Auth)
			}
		})
	}
}

// TestBookingCreateAssemblesBody proves the core write path builds the exact
// v2 create-booking body (eventTypeId int, start, attendee object), and that
// optional notes/metadata are only present when supplied.
func TestBookingCreateAssemblesBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"POST /bookings": {status: 201, body: `{"status":"success","data":{"uid":"bk1"}}`},
	})
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"booking", "create",
		"--event-type-id", "42",
		"--start", "2026-01-01T09:00:00Z",
		"--attendee-name", "Ada",
		"--attendee-email", "ada@example.com",
		"--attendee-tz", "America/New_York",
		"--notes", "prep call",
		"--metadata", `{"source":"helio"}`,
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	req := findReq(reqs, "POST", "/bookings")
	if req == nil {
		t.Fatal("no POST /bookings request")
	}
	if req.Version != "2024-08-13" {
		t.Errorf("version = %q, want 2024-08-13", req.Version)
	}
	body := bodyMap(t, req.Body)
	if got, ok := body["eventTypeId"].(float64); !ok || int(got) != 42 {
		t.Errorf("eventTypeId = %v, want numeric 42", body["eventTypeId"])
	}
	if body["start"] != "2026-01-01T09:00:00Z" {
		t.Errorf("start = %v", body["start"])
	}
	att, ok := body["attendee"].(map[string]any)
	if !ok {
		t.Fatalf("attendee not an object: %v", body["attendee"])
	}
	if att["name"] != "Ada" || att["email"] != "ada@example.com" || att["timeZone"] != "America/New_York" {
		t.Errorf("attendee = %+v", att)
	}
	bfr, ok := body["bookingFieldsResponses"].(map[string]any)
	if !ok || bfr["notes"] != "prep call" {
		t.Errorf("bookingFieldsResponses = %+v, want notes=prep call", body["bookingFieldsResponses"])
	}
	md, ok := body["metadata"].(map[string]any)
	if !ok || md["source"] != "helio" {
		t.Errorf("metadata = %+v", body["metadata"])
	}
	if !strings.Contains(stdout, "bk1") {
		t.Errorf("stdout missing unwrapped data: %s", stdout)
	}
}

// TestBookingCreateOmitsOptionalFields proves the minimal create path sends no
// notes/metadata keys when the flags are absent.
func TestBookingCreateOmitsOptionalFields(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"POST /bookings": {status: 201, body: `{"status":"success","data":{"uid":"bk2"}}`},
	})
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"booking", "create",
		"--event-type-id", "7",
		"--start", "2026-02-01T09:00:00Z",
		"--attendee-name", "Bo",
		"--attendee-email", "bo@example.com",
		"--attendee-tz", "UTC",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/bookings").Body)
	if _, ok := body["bookingFieldsResponses"]; ok {
		t.Error("bookingFieldsResponses present without --notes")
	}
	if _, ok := body["metadata"]; ok {
		t.Error("metadata present without --metadata")
	}
}

// TestEnvelopeUnwrap proves list output is the inner `data` array, not the
// full {status,data} envelope.
func TestEnvelopeUnwrap(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /event-types": {status: 200, body: `{"status":"success","data":[{"id":1,"slug":"intro"}]}`},
	})
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "event-type", "list")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &arr); err != nil {
		t.Fatalf("stdout is not the unwrapped data array: %v (%s)", err, stdout)
	}
	if len(arr) != 1 || arr[0]["slug"] != "intro" {
		t.Errorf("unwrapped data = %+v", arr)
	}
}

// TestSlotQueryParams proves the slot range is passed as eventTypeId/start/end
// query params on the correct version.
func TestSlotQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /slots": {status: 200, body: `{"status":"success","data":{}}`},
	})
	defer srv.Close()

	exit, _, stderr := run(t, srv, "slot", "list",
		"--event-type-id", "42",
		"--start", "2026-01-01T00:00:00Z",
		"--end", "2026-01-08T00:00:00Z")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	req := findReq(reqs, "GET", "/slots")
	if req.Query.Get("eventTypeId") != "42" ||
		req.Query.Get("start") != "2026-01-01T00:00:00Z" ||
		req.Query.Get("end") != "2026-01-08T00:00:00Z" {
		t.Errorf("slot query = %v", req.Query)
	}
}

// TestBookingListStatusFilter proves a valid --status forwards as a query param
// and an invalid one is a usage error (exit 2) that never reaches the API.
func TestBookingListStatusFilter(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /bookings": {status: 200, body: `{"status":"success","data":[]}`},
	})
	defer srv.Close()

	exit, _, _ := run(t, srv, "booking", "list", "--status", "upcoming")
	if exit != 0 {
		t.Fatalf("valid status exit = %d", exit)
	}
	if q := findReq(reqs, "GET", "/bookings").Query.Get("status"); q != "upcoming" {
		t.Errorf("status query = %q", q)
	}

	exit2, _, stderr := run(t, srv, "booking", "list", "--status", "bogus")
	if exit2 != 2 {
		t.Errorf("invalid status exit = %d, want 2", exit2)
	}
	if !strings.Contains(stderr, "upcoming|past|cancelled") {
		t.Errorf("stderr missing enum hint: %s", stderr)
	}
}

// TestMissingRequiredFlag proves a missing required flag is a usage error
// (exit 2) rendered before any request.
func TestMissingRequiredFlag(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, nil)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "event-type", "get")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "--id is required") {
		t.Errorf("stderr = %s", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("expected no API request, got %d", len(reqs))
	}
}

// TestAPIErrorExitAndJSON proves a Cal.com non-2xx maps to exit 1 with the
// error body surfaced, and --json renders the structured api-error envelope.
func TestAPIErrorExitAndJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /bookings/missing": {status: 404, body: `{"status":"error","error":{"code":"NOT_FOUND","message":"no such booking"}}`},
	})
	defer srv.Close()

	exit, _, stderr := run(t, srv, "booking", "get", "--uid", "missing")
	if exit != 1 {
		t.Errorf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "no such booking") {
		t.Errorf("stderr missing provider message: %s", stderr)
	}

	exitJSON, _, stderrJSON := run(t, srv, "booking", "get", "--uid", "missing", "--json")
	if exitJSON != 1 {
		t.Errorf("json exit = %d, want 1", exitJSON)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderrJSON)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%s)", err, stderrJSON)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Errorf("error envelope = %+v", env.Error)
	}
}

// TestUnauthorizedRejectsCredential proves a 401 is classified as a credential
// rejection so the engine can invalidate the token.
func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /me": {status: 401, body: `{"status":"error","error":{"code":"UNAUTHORIZED","message":"bad token"}}`},
	})
	defer srv.Close()

	result, _, _ := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 was not classified as a credential rejection")
	}
}

// TestMissingToken proves a missing CALCOM_TOKEN fails fast (exit 1) before any
// command parsing.
func TestMissingToken(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "CALCOM_TOKEN is not set") {
		t.Errorf("stderr = %s", errBuf.String())
	}
}

// TestUnknownSubcommandIsUsageError proves an unknown subcommand under a group
// is exit 2, not a false success.
func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, nil)
	defer srv.Close()

	exit, _, _ := run(t, srv, "booking", "frobnicate")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
}
