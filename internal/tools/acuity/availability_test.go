package acuity

import (
	"net/http"
	"testing"
)

func TestAvailabilityDates(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"date":"2026-07-15"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"availability", "dates",
		"--type-id", "88",
		"--month", "2026-07",
		"--calendar-id", "55",
		"--timezone", "America/New_York",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/availability/dates" {
		t.Fatalf("request = %s %s, want GET /availability/dates", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "appointmentTypeID", "88")
	assertQuery(t, q, "month", "2026-07")
	assertQuery(t, q, "calendarID", "55")
	assertQuery(t, q, "timezone", "America/New_York")
}

func TestAvailabilityTimes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"time":"2026-07-15T09:00:00-0400"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"availability", "times",
		"--type-id", "88",
		"--date", "2026-07-15",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/availability/times" {
		t.Fatalf("request = %s %s, want GET /availability/times", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "appointmentTypeID", "88")
	assertQuery(t, q, "date", "2026-07-15")
}

func TestAvailabilityDatesRequiresTypeAndMonth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "availability", "dates", "--type-id", "88")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --month", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("usage error must not reach the API")
	}
}

func TestAvailabilityTimesRequiresTypeAndDate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "availability", "times", "--date", "2026-07-15")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --type-id", result.ExitCode)
	}
}
