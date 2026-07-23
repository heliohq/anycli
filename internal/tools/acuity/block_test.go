package acuity

import (
	"net/http"
	"testing"
)

func TestBlockList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"id":9}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"block", "list",
		"--min-date", "2026-07-01",
		"--max-date", "2026-07-31",
		"--calendar-id", "55",
		"--max", "10",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/blocks" {
		t.Fatalf("request = %s %s, want GET /blocks", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "minDate", "2026-07-01")
	assertQuery(t, q, "maxDate", "2026-07-31")
	assertQuery(t, q, "calendarID", "55")
	assertQuery(t, q, "max", "10")
}

func TestBlockCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":9}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"block", "create",
		"--start", "2026-07-18T13:00:00-0400",
		"--end", "2026-07-18T17:00:00-0400",
		"--calendar-id", "55",
		"--notes", "offsite",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/blocks" {
		t.Fatalf("request = %s %s, want POST /blocks", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["start"] != "2026-07-18T13:00:00-0400" || body["end"] != "2026-07-18T17:00:00-0400" {
		t.Errorf("start/end wrong: %v", body)
	}
	if body["calendarID"].(float64) != 55 {
		t.Errorf("calendarID = %v, want 55", body["calendarID"])
	}
	if body["notes"] != "offsite" {
		t.Errorf("notes = %v", body["notes"])
	}
}

func TestBlockDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "block", "delete", "9")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/blocks/9" {
		t.Fatalf("request = %s %s, want DELETE /blocks/9", got.Method, got.Path)
	}
}

func TestBlockCreateRequiresStartEnd(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "block", "create", "--start", "2026-07-18T13:00:00-0400")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --end", result.ExitCode)
	}
}
