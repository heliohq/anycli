package sheets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValuesGet_SingleRange(t *testing.T) {
	body := `{"range":"Sheet1!A1:B2","majorDimension":"ROWS","values":[["a","b"],["c","d"]]}`
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1/values/Sheet1!A1:B2": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "values", "get", "id1", "--range", "Sheet1!A1:B2")
	if !strings.Contains(stdout, "a\tb") || !strings.Contains(stdout, "c\td") {
		t.Errorf("human output = %q, want tab-separated rows", stdout)
	}
	// --json passes the raw provider body through.
	stdout = f.runOK(t, "values", "get", "id1", "--range", "Sheet1!A1:B2", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want raw provider body", stdout)
	}
}

func TestValuesGet_RenderOption(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1/values/A1": {http.StatusOK, `{"range":"A1","values":[["=SUM(1,2)"]]}`},
	})
	f.runOK(t, "values", "get", "id1", "--range", "A1", "--render", "formula")
	got := f.last(t, "GET", "/v4/spreadsheets/id1/values/A1")
	if !strings.Contains(got.Query, "valueRenderOption=FORMULA") {
		t.Errorf("query = %q, want valueRenderOption=FORMULA", got.Query)
	}
}

func TestValuesGet_MultiRangeBatchGet(t *testing.T) {
	body := `{"valueRanges":[{"range":"A1:A1","values":[["x"]]},{"range":"B1:B1","values":[["y"]]}]}`
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1/values:batchGet": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "values", "get", "id1", "--range", "A1:A1", "--range", "B1:B1")
	if !strings.Contains(stdout, "x") || !strings.Contains(stdout, "y") {
		t.Errorf("output = %q, want both ranges", stdout)
	}
	got := f.last(t, "GET", "/v4/spreadsheets/id1/values:batchGet")
	if !strings.Contains(got.Query, "ranges=A1%3AA1") || !strings.Contains(got.Query, "ranges=B1%3AB1") {
		t.Errorf("query = %q, want both ranges as repeated params", got.Query)
	}
}

func TestValuesUpdate_ValueInputOptionAndBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /v4/spreadsheets/id1/values/Sheet1!A1": {http.StatusOK, `{"updatedRange":"Sheet1!A1:B1","updatedCells":2}`},
	})
	stdout := f.runOK(t, "values", "update", "id1", "--range", "Sheet1!A1", "--values-json", `[["hi",7]]`)
	if !strings.Contains(stdout, "updated 2 cell(s) in Sheet1!A1:B1") {
		t.Errorf("output = %q, want update summary", stdout)
	}
	got := f.last(t, "PUT", "/v4/spreadsheets/id1/values/Sheet1!A1")
	if !strings.Contains(got.Query, "valueInputOption=USER_ENTERED") {
		t.Errorf("query = %q, want default USER_ENTERED", got.Query)
	}
	var vr valueRange
	if err := json.Unmarshal(got.Body, &vr); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if vr.MajorDimension != "ROWS" || len(vr.Values) != 1 || len(vr.Values[0]) != 2 {
		t.Errorf("body = %+v, want one ROWS row of two cells", vr)
	}
}

func TestValuesUpdate_RawFlag(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /v4/spreadsheets/id1/values/A1": {http.StatusOK, `{"updatedRange":"A1","updatedCells":1}`},
	})
	f.runOK(t, "values", "update", "id1", "--range", "A1", "--values-json", `[["=A2"]]`, "--raw")
	got := f.last(t, "PUT", "/v4/spreadsheets/id1/values/A1")
	if !strings.Contains(got.Query, "valueInputOption=RAW") {
		t.Errorf("query = %q, want RAW with --raw", got.Query)
	}
}

func TestValuesUpdate_CSVFile(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "grid.csv")
	if err := os.WriteFile(csvPath, []byte("name,score\nalice,10\nbob,20\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"PUT /v4/spreadsheets/id1/values/A1": {http.StatusOK, `{"updatedRange":"A1:B3","updatedCells":6}`},
	})
	f.runOK(t, "values", "update", "id1", "--range", "A1", "--csv-file", csvPath)
	got := f.last(t, "PUT", "/v4/spreadsheets/id1/values/A1")
	var vr valueRange
	if err := json.Unmarshal(got.Body, &vr); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(vr.Values) != 3 || len(vr.Values[0]) != 2 {
		t.Errorf("csv body = %+v, want 3 rows of 2 cells", vr.Values)
	}
}

func TestValuesAppend(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1/values/Log!A1:append": {http.StatusOK, `{"updates":{"updatedRange":"Log!A5:B5","updatedCells":2}}`},
	})
	stdout := f.runOK(t, "values", "append", "id1", "--range", "Log!A1", "--values-json", `[["2026-07-16","done"]]`)
	if !strings.Contains(stdout, "appended 2 cell(s) at Log!A5:B5") {
		t.Errorf("output = %q, want append summary", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1/values/Log!A1:append")
	if !strings.Contains(got.Query, "valueInputOption=USER_ENTERED") {
		t.Errorf("query = %q, want USER_ENTERED", got.Query)
	}
}

func TestValuesClear_Single(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1/values/Sheet1!A1:B2:clear": {http.StatusOK, `{"clearedRange":"Sheet1!A1:B2"}`},
	})
	stdout := f.runOK(t, "values", "clear", "id1", "--range", "Sheet1!A1:B2")
	if !strings.Contains(stdout, "cleared Sheet1!A1:B2") {
		t.Errorf("output = %q, want clear summary", stdout)
	}
}

func TestValuesClear_MultiBatchClear(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1/values:batchClear": {http.StatusOK, `{"clearedRanges":["A1:A2","B1:B2"]}`},
	})
	stdout := f.runOK(t, "values", "clear", "id1", "--range", "A1:A2", "--range", "B1:B2")
	if !strings.Contains(stdout, "cleared 2 range(s)") {
		t.Errorf("output = %q, want batchClear summary", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1/values:batchClear")
	var payload struct {
		Ranges []string `json:"ranges"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload.Ranges) != 2 {
		t.Errorf("body ranges = %v, want 2", payload.Ranges)
	}
}
