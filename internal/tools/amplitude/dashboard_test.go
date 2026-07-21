package amplitude

import (
	"net/http"
	"strings"
	"testing"
)

// (c/d) Each subcommand maps to the documented path and query params, and the
// JSON e=/s= values are URL-encoded (Go's url.Values.Encode handles this).
func TestSegmentationPathAndParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()

	res, stdout, stderr := run(t, srv,
		"segmentation",
		"--events", `{"event_type":"Add to Cart"}`,
		"--start", "20220101", "--end", "20220107",
		"--metric", "totals", "--interval", "7",
		"--segment", `[{"prop":"country","op":"is","values":["US"]}]`,
	)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Path != "/api/2/events/segmentation" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("e") != `{"event_type":"Add to Cart"}` {
		t.Errorf("e = %q", q.Get("e"))
	}
	if q.Get("start") != "20220101" || q.Get("end") != "20220107" {
		t.Errorf("start/end = %q/%q", q.Get("start"), q.Get("end"))
	}
	if q.Get("m") != "totals" || q.Get("i") != "7" {
		t.Errorf("m/i = %q/%q", q.Get("m"), q.Get("i"))
	}
	if q.Get("s") != `[{"prop":"country","op":"is","values":["US"]}]` {
		t.Errorf("s = %q", q.Get("s"))
	}
	// The raw query must be percent-encoded (no literal spaces/braces on the wire).
	if strings.ContainsAny(got.Query, "{} ") {
		t.Errorf("raw query not URL-encoded: %q", got.Query)
	}
	if stdout != "{\"data\":{}}\n" {
		t.Errorf("stdout = %q, want verbatim JSON + newline", stdout)
	}
}

func TestSegmentationRequiresEvents(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "segmentation", "--start", "20220101", "--end", "20220107")
	if res.ExitCode != 2 {
		t.Errorf("missing --events exit = %d, want 2", res.ExitCode)
	}
}

func TestSegmentationInvalidJSONExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "segmentation", "--events", "{not json", "--start", "20220101", "--end", "20220107")
	if res.ExitCode != 2 {
		t.Errorf("invalid --events JSON exit = %d, want 2", res.ExitCode)
	}
}

// Funnels sends one `e` per step (repeatable, ordered).
func TestFunnelsMultipleEvents(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	res, _, stderr := run(t, srv,
		"funnels",
		"--events", `{"event_type":"View"}`,
		"--events", `{"event_type":"Purchase"}`,
		"--start", "20220101", "--end", "20220107",
	)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Path != "/api/2/funnels" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	es := q["e"]
	if len(es) != 2 || es[0] != `{"event_type":"View"}` || es[1] != `{"event_type":"Purchase"}` {
		t.Errorf("e params = %#v", es)
	}
}

func TestFunnelsRequiresTwoEvents(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "funnels", "--events", `{"event_type":"View"}`, "--start", "20220101", "--end", "20220107")
	if res.ExitCode != 2 {
		t.Errorf("single-step funnel exit = %d, want 2", res.ExitCode)
	}
}

// Retention uses se/re (not e/s).
func TestRetentionStartReturningEvents(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	res, _, stderr := run(t, srv,
		"retention",
		"--start-event", `{"event_type":"Signup"}`,
		"--returning-event", `{"event_type":"Open"}`,
		"--start", "20220101", "--end", "20220107", "--interval", "7",
	)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Path != "/api/2/retention" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("se") != `{"event_type":"Signup"}` || q.Get("re") != `{"event_type":"Open"}` {
		t.Errorf("se/re = %q/%q", q.Get("se"), q.Get("re"))
	}
	if q.Get("i") != "7" {
		t.Errorf("i = %q", q.Get("i"))
	}
}

func TestEventsListPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "events", "list")
	if res.ExitCode != 0 || got.Path != "/api/2/events/list" {
		t.Errorf("events list: exit=%d path=%q", res.ExitCode, got.Path)
	}
}

func TestUserSearchPathAndParam(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"matches":[]}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "user-search", "--user", "alice@example.com")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	if got.Path != "/api/2/usersearch" {
		t.Errorf("path = %q", got.Path)
	}
	if parseQuery(t, got.Query).Get("user") != "alice@example.com" {
		t.Errorf("user = %q", got.Query)
	}
}

func TestUserActivityPathAndParam(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"events":[]}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "user-activity", "--user", "123456789", "--limit", "100")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	if got.Path != "/api/2/useractivity" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("user") != "123456789" || q.Get("limit") != "100" {
		t.Errorf("user/limit = %q/%q", q.Get("user"), q.Get("limit"))
	}
}

func TestUserActivityRequiresUser(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "user-activity")
	if res.ExitCode != 2 {
		t.Errorf("missing --user exit = %d, want 2", res.ExitCode)
	}
}

func TestCohortsListPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"cohorts":[]}`, &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "cohorts", "list")
	if res.ExitCode != 0 || got.Path != "/api/3/cohorts" {
		t.Errorf("cohorts list: exit=%d path=%q", res.ExitCode, got.Path)
	}
}
