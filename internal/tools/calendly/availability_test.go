package calendly

import (
	"net/http"
	"testing"
)

func TestAvailabilitySlotsPassesRangeUnmodified(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"collection":[]}`, &got)
	defer srv.Close()

	// A 31-day span the API would reject; the tool must pass it through
	// unmodified (no client-side range rejection).
	code, _, stderr := run(t, srv,
		"availability", "slots",
		"--event-type", "ET-1",
		"--from", "2026-08-01T00:00:00Z",
		"--to", "2026-09-01T00:00:00Z",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Path != "/event_type_available_times" {
		t.Fatalf("path = %q, want /event_type_available_times", got.Path)
	}
	q := parseQuery(t, got.Query)
	if want := srv.URL + "/event_types/ET-1"; q.Get("event_type") != want {
		t.Errorf("event_type = %q, want %q", q.Get("event_type"), want)
	}
	if q.Get("start_time") != "2026-08-01T00:00:00Z" {
		t.Errorf("start_time = %q, want passthrough", q.Get("start_time"))
	}
	if q.Get("end_time") != "2026-09-01T00:00:00Z" {
		t.Errorf("end_time = %q, want passthrough (no clamp)", q.Get("end_time"))
	}
}

func TestAvailabilitySlotsRequiresFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "availability", "slots", "--event-type", "ET-1")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for missing required --from/--to", code)
	}
}

func TestAvailabilityBusyResolvesMe(t *testing.T) {
	routes := map[string]routeHandler{
		"/users/me":        {http.StatusOK, meResponse("BASE")},
		"/user_busy_times": {http.StatusOK, `{"collection":[]}`},
	}
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	routes["/users/me"] = routeHandler{http.StatusOK, meResponse(srv.URL)}

	code, _, stderr := run(t, srv,
		"availability", "busy",
		"--from", "2026-08-01T00:00:00Z",
		"--to", "2026-08-05T00:00:00Z",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	q := parseQuery(t, captured["/user_busy_times"].Query)
	if q.Get("user") != srv.URL+"/users/ME" {
		t.Errorf("user = %q, want resolved me uri", q.Get("user"))
	}
	if q.Get("start_time") == "" || q.Get("end_time") == "" {
		t.Errorf("start_time/end_time missing: %q", captured["/user_busy_times"].Query)
	}
}

func TestAvailabilityScheduleList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"collection":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "availability", "schedule", "list", "--user", "U-7")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Path != "/user_availability_schedules" {
		t.Fatalf("path = %q, want /user_availability_schedules", got.Path)
	}
	q := parseQuery(t, got.Query)
	if want := srv.URL + "/users/U-7"; q.Get("user") != want {
		t.Errorf("user = %q, want %q", q.Get("user"), want)
	}
}
