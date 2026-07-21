package semrush

import (
	"net/http"
	"testing"
)

const domainOrganicCSV = "Keyword;Position;Search Volume;CPC;Traffic (%)\n" +
	"seo tools;3;74000;1.50;12.5\n" +
	"keyword research;5;40000;2.10;8.0\n"

func TestDomainOrganic_QueryAndCSVMapping(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, domainOrganicCSV, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "domain", "organic", "example.com")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	q := parseQuery(t, got.Query)
	if q.Get("key") != "key-abcd1234" {
		t.Errorf("key = %q, want the injected key", q.Get("key"))
	}
	if q.Get("type") != "domain_organic" {
		t.Errorf("type = %q, want domain_organic", q.Get("type"))
	}
	if q.Get("domain") != "example.com" {
		t.Errorf("domain = %q", q.Get("domain"))
	}
	if q.Get("database") != "us" {
		t.Errorf("database = %q, want default us", q.Get("database"))
	}
	if q.Get("display_limit") != "10" {
		t.Errorf("display_limit = %q, want default 10", q.Get("display_limit"))
	}

	env := decodeEnvelope(t, stdout)
	if env.Report != "domain_organic" || env.Database != "us" || env.RowCount != 2 {
		t.Fatalf("envelope = %+v", env)
	}
	first := env.Rows[0]
	if first["keyword"] != "seo tools" {
		t.Errorf("keyword = %v, want snake_cased header mapping", first["keyword"])
	}
	// Numeric coercion: integers stay integers, decimals become floats.
	if v, ok := first["position"].(float64); !ok || v != 3 {
		t.Errorf("position = %v (%T), want numeric 3", first["position"], first["position"])
	}
	if v, ok := first["search_volume"].(float64); !ok || v != 74000 {
		t.Errorf("search_volume = %v (%T), want numeric 74000", first["search_volume"], first["search_volume"])
	}
	if v, ok := first["cpc"].(float64); !ok || v != 1.5 {
		t.Errorf("cpc = %v (%T), want numeric 1.5", first["cpc"], first["cpc"])
	}
	if _, ok := first["traffic"]; !ok {
		t.Errorf("expected snake_cased 'traffic' key from 'Traffic (%%)', got row %v", first)
	}
}

func TestDomainOverview_AllDatabasesDropsDatabaseParam(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Database;Domain;Rank\nus;example.com;5\n", &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "domain", "overview", "example.com", "--all-databases")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("type") != "domain_ranks" {
		t.Errorf("type = %q, want domain_ranks for --all-databases", q.Get("type"))
	}
	if _, present := q["database"]; present {
		t.Errorf("database param must be absent for --all-databases, query=%q", got.Query)
	}
	env := decodeEnvelope(t, stdout)
	if env.Database != "" {
		t.Errorf("envelope database = %q, want empty for all-databases", env.Database)
	}
}

func TestDomainOverview_SingleDatabaseDefault(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Domain;Rank\nexample.com;5\n", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "overview", "example.com")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("type") != "domain_rank" {
		t.Errorf("type = %q, want domain_rank (single db)", q.Get("type"))
	}
	if q.Get("database") != "us" {
		t.Errorf("database = %q, want us", q.Get("database"))
	}
}

func TestDomainCompetitors_PaidSwitchesType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Domain;Competitor Relevance\nrival.com;0.8\n", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "competitors", "example.com", "--paid")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := parseQuery(t, got.Query).Get("type"); got != "domain_adwords_adwords" {
		t.Errorf("type = %q, want domain_adwords_adwords for --paid", got)
	}
}

func TestKeywordBatch_JoinsPhrasesWithSemicolon(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Keyword;Search Volume\nfoo;10\nbar;20\n", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keyword", "batch", "foo", "bar", "--database", "uk")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("type") != "phrase_these" {
		t.Errorf("type = %q, want phrase_these", q.Get("type"))
	}
	if q.Get("phrase") != "foo;bar" {
		t.Errorf("phrase = %q, want foo;bar", q.Get("phrase"))
	}
	if q.Get("database") != "uk" {
		t.Errorf("database = %q, want uk", q.Get("database"))
	}
}

func TestBacklinks_UsesAnalyticsBaseAndTargetType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "source_url;target_url\nhttp://a;http://b\n", &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "backlinks", "list", "example.com", "--limit", "25")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/analytics/v1/" {
		t.Errorf("path = %q, want /analytics/v1/ for backlinks", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("type") != "backlinks" {
		t.Errorf("type = %q, want backlinks", q.Get("type"))
	}
	if q.Get("target") != "example.com" {
		t.Errorf("target = %q", q.Get("target"))
	}
	if q.Get("target_type") != "root_domain" {
		t.Errorf("target_type = %q, want default root_domain", q.Get("target_type"))
	}
	if _, present := q["database"]; present {
		t.Errorf("backlinks reports must not send a database param, query=%q", got.Query)
	}
	if q.Get("display_limit") != "25" {
		t.Errorf("display_limit = %q, want 25", q.Get("display_limit"))
	}
	env := decodeEnvelope(t, stdout)
	if env.Database != "" {
		t.Errorf("envelope database = %q, want empty for backlinks", env.Database)
	}
}

func TestReport_OptionalFlagsOmittedWhenUnset(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Keyword\nfoo\n", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keyword", "overview", "foo")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	q := parseQuery(t, got.Query)
	for _, absent := range []string{"display_offset", "export_columns", "display_filter", "display_sort", "display_date", "display_positions"} {
		if _, present := q[absent]; present {
			t.Errorf("%s should be omitted when unset, query=%q", absent, got.Query)
		}
	}
}

func TestReport_OptionalFlagsForwarded(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "Keyword\nfoo\n", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "keyword", "organic-results", "foo",
		"--limit", "50", "--offset", "10", "--columns", "Dn,Ur",
		"--filter", "+|Nq|Gt|100", "--sort", "tr_desc", "--date", "20260115", "--positions", "new")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	q := parseQuery(t, got.Query)
	checks := map[string]string{
		"display_limit":     "50",
		"display_offset":    "10",
		"export_columns":    "Dn,Ur",
		"display_filter":    "+|Nq|Gt|100",
		"display_sort":      "tr_desc",
		"display_date":      "20260115",
		"display_positions": "new",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
}

func TestReport_MissingArgIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "", &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "domain", "organic")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP request should be made on a usage error, saw %s", got.Method)
	}
	if stderr == "" {
		t.Error("expected a usage error on stderr")
	}
}

func TestUnknownSubcommand_UsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "", &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "domain", "bogus", "example.com")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", code)
	}
}
