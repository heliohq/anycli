package googleanalytics

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const runReportBody = `{
  "dimensionHeaders": [{"name": "country"}],
  "metricHeaders": [{"name": "activeUsers", "type": "TYPE_INTEGER"}],
  "rows": [
    {"dimensionValues": [{"value": "United States"}], "metricValues": [{"value": "120"}]},
    {"dimensionValues": [{"value": "Germany"}], "metricValues": [{"value": "31"}]}
  ],
  "rowCount": 2,
  "kind": "analyticsData#runReport"
}`

// decodeBody unmarshals a recorded request body for structural assertions.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body is not JSON: %v (%q)", err, raw)
	}
	return m
}

func TestReportRunRequestShape(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runReport": {status: http.StatusOK, body: runReportBody},
	})
	f.runOK(t, "report", "run",
		"--property", "123456",
		"--metrics", "activeUsers,sessions",
		"--dimensions", "country,city",
	)

	req := f.last(t, "POST", "/data/v1beta/properties/123456:runReport")
	if req.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the injected bearer token", req.Auth)
	}
	body := decodeBody(t, req.Body)

	metrics, _ := json.Marshal(body["metrics"])
	if string(metrics) != `[{"name":"activeUsers"},{"name":"sessions"}]` {
		t.Errorf("metrics = %s", metrics)
	}
	dims, _ := json.Marshal(body["dimensions"])
	if string(dims) != `[{"name":"country"},{"name":"city"}]` {
		t.Errorf("dimensions = %s", dims)
	}
	ranges, _ := json.Marshal(body["dateRanges"])
	if string(ranges) != `[{"endDate":"today","startDate":"28daysAgo"}]` {
		t.Errorf("dateRanges = %s, want the 28daysAgo..today default", ranges)
	}
	for _, absent := range []string{"dimensionFilter", "orderBys", "limit", "offset"} {
		if _, ok := body[absent]; ok {
			t.Errorf("body carries %q without the flag being set", absent)
		}
	}
}

func TestReportRunPropertyPrefixNormalized(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runReport": {status: http.StatusOK, body: runReportBody},
	})
	f.runOK(t, "report", "run", "--property", "properties/123456", "--metrics", "activeUsers")
	f.last(t, "POST", "/data/v1beta/properties/123456:runReport")
}

func TestReportRunInvalidPropertyIsUsageError(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "report", "run", "--property", "acme.com", "--metrics", "activeUsers")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for a non-numeric property", result.ExitCode)
	}
	if !strings.Contains(stderr, "property") {
		t.Errorf("stderr = %q, want a property usage error", stderr)
	}
}

func TestReportRunMissingMetricsIsUsageError(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, _ := f.run(t, "report", "run", "--property", "123456")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for missing --metrics", result.ExitCode)
	}
}

func TestReportRunDatesFiltersOrderingPagination(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/9:runReport": {status: http.StatusOK, body: runReportBody},
	})
	f.runOK(t, "report", "run",
		"--property", "9",
		"--metrics", "activeUsers",
		"--dimensions", "country",
		"--start-date", "2026-06-01",
		"--end-date", "yesterday",
		"--filter", "country==Germany",
		"--filter", "city==Berlin",
		"--order-by", "metric:activeUsers:desc",
		"--order-by", "dimension:country:asc",
		"--limit", "10",
		"--offset", "20",
	)

	body := decodeBody(t, f.last(t, "POST", "/data/v1beta/properties/9:runReport").Body)

	ranges, _ := json.Marshal(body["dateRanges"])
	if string(ranges) != `[{"endDate":"yesterday","startDate":"2026-06-01"}]` {
		t.Errorf("dateRanges = %s", ranges)
	}
	filter, _ := json.Marshal(body["dimensionFilter"])
	want := `{"andGroup":{"expressions":[` +
		`{"filter":{"fieldName":"country","stringFilter":{"matchType":"EXACT","value":"Germany"}}},` +
		`{"filter":{"fieldName":"city","stringFilter":{"matchType":"EXACT","value":"Berlin"}}}]}}`
	if string(filter) != want {
		t.Errorf("dimensionFilter = %s, want %s", filter, want)
	}
	orderBys, _ := json.Marshal(body["orderBys"])
	wantOrder := `[{"desc":true,"metric":{"metricName":"activeUsers"}},{"dimension":{"dimensionName":"country"}}]`
	if string(orderBys) != wantOrder {
		t.Errorf("orderBys = %s, want %s", orderBys, wantOrder)
	}
	if body["limit"] != float64(10) || body["offset"] != float64(20) {
		t.Errorf("limit/offset = %v/%v, want 10/20", body["limit"], body["offset"])
	}
}

func TestReportRunSingleFilterIsPlainExpression(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/9:runReport": {status: http.StatusOK, body: runReportBody},
	})
	f.runOK(t, "report", "run", "--property", "9", "--metrics", "activeUsers",
		"--filter", "country==Germany")

	body := decodeBody(t, f.last(t, "POST", "/data/v1beta/properties/9:runReport").Body)
	filter, _ := json.Marshal(body["dimensionFilter"])
	want := `{"filter":{"fieldName":"country","stringFilter":{"matchType":"EXACT","value":"Germany"}}}`
	if string(filter) != want {
		t.Errorf("dimensionFilter = %s, want %s", filter, want)
	}
}

func TestReportRunFilterJSONPassesRawExpression(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/9:runReport": {status: http.StatusOK, body: runReportBody},
	})
	raw := `{"notExpression":{"filter":{"fieldName":"country","inListFilter":{"values":["US","DE"]}}}}`
	f.runOK(t, "report", "run", "--property", "9", "--metrics", "activeUsers", "--filter-json", raw)

	body := decodeBody(t, f.last(t, "POST", "/data/v1beta/properties/9:runReport").Body)
	filter, _ := json.Marshal(body["dimensionFilter"])
	if string(filter) != raw {
		t.Errorf("dimensionFilter = %s, want the raw expression verbatim", filter)
	}
}

func TestReportRunFilterFlagConflictsAreUsageErrors(t *testing.T) {
	f := newFixture(t, map[string]route{})
	cases := [][]string{
		{"report", "run", "--property", "9", "--metrics", "m", "--filter", "a==b", "--filter-json", `{}`},
		{"report", "run", "--property", "9", "--metrics", "m", "--filter", "no-equals"},
		{"report", "run", "--property", "9", "--metrics", "m", "--filter-json", `{not json`},
		{"report", "run", "--property", "9", "--metrics", "m", "--order-by", "bogus:activeUsers"},
		{"report", "run", "--property", "9", "--metrics", "m", "--order-by", "metric:activeUsers:sideways"},
	}
	for _, args := range cases {
		result, _, _ := f.run(t, args...)
		if result.ExitCode != 2 {
			t.Errorf("args %v: exit code = %d, want 2", args, result.ExitCode)
		}
	}
}

func TestReportRunTableOutput(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runReport": {status: http.StatusOK, body: runReportBody},
	})
	stdout := f.runOK(t, "report", "run", "--property", "123456", "--metrics", "activeUsers", "--dimensions", "country")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout = %q, want a header plus two rows", stdout)
	}
	if lines[0] != "country\tactiveUsers" {
		t.Errorf("header = %q", lines[0])
	}
	if lines[1] != "United States\t120" || lines[2] != "Germany\t31" {
		t.Errorf("rows = %q", lines[1:])
	}
}

func TestReportRunNoRows(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runReport": {status: http.StatusOK,
			body: `{"metricHeaders":[{"name":"activeUsers"}],"rowCount":0}`},
	})
	stdout := f.runOK(t, "report", "run", "--property", "123456", "--metrics", "activeUsers")
	if !strings.Contains(stdout, "no rows") {
		t.Errorf("stdout = %q, want the no-rows message", stdout)
	}
}

func TestReportRunJSONPassthrough(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runReport": {status: http.StatusOK, body: runReportBody},
	})
	stdout := f.runOK(t, "report", "run", "--property", "123456", "--metrics", "activeUsers", "--json")
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout is not valid JSON: %q", stdout)
	}
	if !strings.Contains(stdout, `"analyticsData#runReport"`) {
		t.Errorf("stdout = %q, want the provider body", stdout)
	}
}

func TestReportRealtime(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/123456:runRealtimeReport": {status: http.StatusOK,
			body: `{"metricHeaders":[{"name":"activeUsers"}],"rows":[{"metricValues":[{"value":"7"}]}],"rowCount":1}`},
	})
	stdout := f.runOK(t, "report", "realtime", "--property", "properties/123456", "--metrics", "activeUsers")

	body := decodeBody(t, f.last(t, "POST", "/data/v1beta/properties/123456:runRealtimeReport").Body)
	metrics, _ := json.Marshal(body["metrics"])
	if string(metrics) != `[{"name":"activeUsers"}]` {
		t.Errorf("metrics = %s", metrics)
	}
	if _, ok := body["minuteRanges"]; ok {
		t.Error("minuteRanges present without --minutes-ago")
	}
	if _, ok := body["dateRanges"]; ok {
		t.Error("realtime body must not carry dateRanges")
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if lines[0] != "activeUsers" || lines[1] != "7" {
		t.Errorf("stdout = %q, want the metric header and value", stdout)
	}
}

func TestReportRealtimeMinutesAgo(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /data/v1beta/properties/9:runRealtimeReport": {status: http.StatusOK, body: `{"rowCount":0}`},
	})
	f.runOK(t, "report", "realtime", "--property", "9", "--metrics", "activeUsers", "--minutes-ago", "15")

	body := decodeBody(t, f.last(t, "POST", "/data/v1beta/properties/9:runRealtimeReport").Body)
	ranges, _ := json.Marshal(body["minuteRanges"])
	if string(ranges) != `[{"startMinutesAgo":15}]` {
		t.Errorf("minuteRanges = %s", ranges)
	}
}

const metadataBody = `{
  "name": "properties/123456/metadata",
  "dimensions": [
    {"apiName": "country", "uiName": "Country", "category": "Geography"},
    {"apiName": "city", "uiName": "City", "category": "Geography"}
  ],
  "metrics": [
    {"apiName": "activeUsers", "uiName": "Active users", "category": "User"},
    {"apiName": "sessions", "uiName": "Sessions", "category": "Session"}
  ]
}`

func TestReportMetadata(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /data/v1beta/properties/123456/metadata": {status: http.StatusOK, body: metadataBody},
	})
	stdout := f.runOK(t, "report", "metadata", "--property", "123456")
	for _, want := range []string{"dimension\tcountry\tCountry", "metric\tsessions\tSessions"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestReportMetadataKindAndSearch(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /data/v1beta/properties/123456/metadata": {status: http.StatusOK, body: metadataBody},
	})
	stdout := f.runOK(t, "report", "metadata", "--property", "123456", "--kind", "metrics", "--search", "active")
	if strings.Contains(stdout, "country") || strings.Contains(stdout, "sessions") {
		t.Errorf("stdout = %q, want dimensions and non-matching metrics filtered out", stdout)
	}
	if !strings.Contains(stdout, "activeUsers") {
		t.Errorf("stdout = %q, want the matching metric", stdout)
	}
}

func TestReportMetadataBadKindIsUsageError(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, _ := f.run(t, "report", "metadata", "--property", "123456", "--kind", "bogus")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for a bad --kind", result.ExitCode)
	}
}

func TestReportMetadataJSONUnfilteredIsVerbatim(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /data/v1beta/properties/123456/metadata": {status: http.StatusOK, body: metadataBody},
	})
	stdout := f.runOK(t, "report", "metadata", "--property", "123456", "--json")
	var got, want any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	_ = json.Unmarshal([]byte(metadataBody), &want)
	gotB, _ := json.Marshal(got)
	wantB, _ := json.Marshal(want)
	if string(gotB) != string(wantB) {
		t.Errorf("stdout = %s, want the provider body", gotB)
	}
}

func TestReportMetadataJSONFiltered(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /data/v1beta/properties/123456/metadata": {status: http.StatusOK, body: metadataBody},
	})
	stdout := f.runOK(t, "report", "metadata", "--property", "123456", "--kind", "dimensions", "--search", "city", "--json")
	var got struct {
		Dimensions []struct {
			APIName string `json:"apiName"`
		} `json:"dimensions"`
		Metrics []json.RawMessage `json:"metrics"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if len(got.Dimensions) != 1 || got.Dimensions[0].APIName != "city" {
		t.Errorf("dimensions = %+v, want only city", got.Dimensions)
	}
	if len(got.Metrics) != 0 {
		t.Errorf("metrics = %v, want none under --kind dimensions", got.Metrics)
	}
}
