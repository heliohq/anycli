package acuity

import (
	"net/http"
	"testing"
)

func TestAppointmentList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"id":1}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv,
		"appointment", "list",
		"--min-date", "2026-07-01",
		"--max-date", "2026-07-31",
		"--calendar-id", "55",
		"--type-id", "88",
		"--email", "jane@example.com",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--canceled",
		"--exclude-forms",
		"--max", "25",
		"--direction", "ASC",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/appointments" {
		t.Fatalf("request = %s %s, want GET /appointments", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "minDate", "2026-07-01")
	assertQuery(t, q, "maxDate", "2026-07-31")
	assertQuery(t, q, "calendarID", "55")
	assertQuery(t, q, "appointmentTypeID", "88")
	assertQuery(t, q, "email", "jane@example.com")
	assertQuery(t, q, "firstName", "Jane")
	assertQuery(t, q, "lastName", "Doe")
	assertQuery(t, q, "canceled", "true")
	assertQuery(t, q, "excludeForms", "true")
	assertQuery(t, q, "max", "25")
	assertQuery(t, q, "direction", "ASC")
	if stdout != "[{\"id\":1}]\n" {
		t.Errorf("stdout = %q, want passthrough JSON + newline", stdout)
	}
}

func TestAppointmentListOmitsUnsetFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	run(t, srv, "appointment", "list")
	q := parseQuery(t, got.Query)
	for _, key := range []string{"minDate", "maxDate", "calendarID", "appointmentTypeID", "email", "canceled", "excludeForms", "direction"} {
		if q.Has(key) {
			t.Errorf("unset filter %q leaked into query: %s", key, got.Query)
		}
	}
}

func TestAppointmentGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "appointment", "get", "42")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/appointments/42" {
		t.Fatalf("request = %s %s, want GET /appointments/42", got.Method, got.Path)
	}
}

func TestAppointmentCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":7}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"appointment", "create",
		"--type-id", "88",
		"--datetime", "2026-07-15T09:00:00-0400",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--email", "jane@example.com",
		"--phone", "555-1234",
		"--timezone", "America/New_York",
		"--calendar-id", "55",
		"--notes", "VIP",
		"--field", "1=hello",
		"--field", "2=world",
		"--admin",
		"--no-email",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/appointments" {
		t.Fatalf("request = %s %s, want POST /appointments", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	assertQuery(t, q, "admin", "true")
	assertQuery(t, q, "noEmail", "true")

	body := decodeBody(t, got.Body)
	if body["appointmentTypeID"].(float64) != 88 {
		t.Errorf("appointmentTypeID = %v, want 88 (numeric)", body["appointmentTypeID"])
	}
	if body["calendarID"].(float64) != 55 {
		t.Errorf("calendarID = %v, want 55 (numeric)", body["calendarID"])
	}
	if body["datetime"] != "2026-07-15T09:00:00-0400" {
		t.Errorf("datetime = %v", body["datetime"])
	}
	if body["firstName"] != "Jane" || body["lastName"] != "Doe" {
		t.Errorf("name fields wrong: %v", body)
	}
	if body["notes"] != "VIP" {
		t.Errorf("notes = %v", body["notes"])
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 2 {
		t.Fatalf("fields = %v, want 2 entries", body["fields"])
	}
	first := fields[0].(map[string]any)
	if first["id"].(float64) != 1 || first["value"] != "hello" {
		t.Errorf("field[0] = %v, want {id:1,value:hello}", first)
	}
}

func TestAppointmentCreateBadFieldIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	// --field must be id=value with an integer id.
	result, _, _ := runResult(t, srv,
		"appointment", "create",
		"--type-id", "88", "--datetime", "x", "--first-name", "J", "--last-name", "D",
		"--field", "notanint=hi",
	)
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for malformed --field", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("malformed --field must not reach the API")
	}
}

func TestAppointmentCreateOmitsUnsetOptionalBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	run(t, srv, "appointment", "create",
		"--type-id", "88", "--datetime", "x", "--first-name", "J", "--last-name", "D")
	q := parseQuery(t, got.Query)
	if q.Has("admin") || q.Has("noEmail") {
		t.Errorf("unset admin/no-email leaked: %s", got.Query)
	}
	body := decodeBody(t, got.Body)
	for _, key := range []string{"calendarID", "notes", "fields", "phone", "timezone", "email"} {
		if _, present := body[key]; present {
			t.Errorf("unset optional %q leaked into body: %v", key, body)
		}
	}
}

func TestAppointmentReschedule(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"appointment", "reschedule", "42",
		"--datetime", "2026-07-16T10:00:00-0400",
		"--calendar-id", "55",
		"--admin",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/appointments/42/reschedule" {
		t.Fatalf("request = %s %s, want PUT .../reschedule", got.Method, got.Path)
	}
	assertQuery(t, parseQuery(t, got.Query), "admin", "true")
	body := decodeBody(t, got.Body)
	if body["datetime"] != "2026-07-16T10:00:00-0400" {
		t.Errorf("datetime = %v", body["datetime"])
	}
	if body["calendarID"].(float64) != 55 {
		t.Errorf("calendarID = %v, want 55", body["calendarID"])
	}
}

func TestAppointmentCancel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"appointment", "cancel", "42",
		"--note", "client asked to cancel",
		"--no-email",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/appointments/42/cancel" {
		t.Fatalf("request = %s %s, want PUT .../cancel", got.Method, got.Path)
	}
	assertQuery(t, parseQuery(t, got.Query), "noEmail", "true")
	body := decodeBody(t, got.Body)
	if body["cancelNote"] != "client asked to cancel" {
		t.Errorf("cancelNote = %v", body["cancelNote"])
	}
}

func TestAppointmentUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":42}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"appointment", "update", "42",
		"--email", "new@example.com",
		"--notes", "rescheduled by phone",
		"--field", "3=updated",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/appointments/42" {
		t.Fatalf("request = %s %s, want PUT /appointments/42", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "new@example.com" {
		t.Errorf("email = %v", body["email"])
	}
	if _, present := body["firstName"]; present {
		t.Errorf("unset firstName must not appear in update body: %v", body)
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("fields = %v, want 1 entry", body["fields"])
	}
}

// assertQuery fails unless q[key] == want.
func assertQuery(t *testing.T, q map[string][]string, key, want string) {
	t.Helper()
	got := ""
	if vs := q[key]; len(vs) > 0 {
		got = vs[0]
	}
	if got != want {
		t.Errorf("query %q = %q, want %q", key, got, want)
	}
}
