package sheets

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// metaBody is the canned spreadsheets.get metadata the tab commands resolve
// against.
const metaBody = `{"spreadsheetId":"id1","properties":{"title":"T"},"sheets":[{"properties":{"sheetId":0,"title":"Sheet1"}},{"properties":{"sheetId":123,"title":"Data"}}]}`

func TestTabsAdd(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{"addSheet":{"properties":{"sheetId":555,"title":"Notes"}}}]}`},
	})
	stdout := f.runOK(t, "tabs", "add", "id1", "--title", "Notes")
	if !strings.Contains(stdout, "added tab \"Notes\" (gid=555)") {
		t.Errorf("output = %q, want added-tab summary with new gid", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	assertRequestType(t, got.Body, "addSheet")
}

func TestTabsRename_ByTitle(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1":              {http.StatusOK, metaBody},
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{}]}`},
	})
	stdout := f.runOK(t, "tabs", "rename", "id1", "--tab", "Data", "--title", "Metrics")
	if !strings.Contains(stdout, "renamed tab gid=123 to \"Metrics\"") {
		t.Errorf("output = %q, want rename summary resolving Data → gid 123", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	req := firstRequest(t, got.Body)
	usp, ok := req["updateSheetProperties"].(map[string]any)
	if !ok {
		t.Fatalf("request = %v, want updateSheetProperties", req)
	}
	if usp["fields"] != "title" {
		t.Errorf("fields = %v, want \"title\"", usp["fields"])
	}
}

func TestTabsDelete_ByGID(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1":              {http.StatusOK, metaBody},
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{}]}`},
	})
	stdout := f.runOK(t, "tabs", "delete", "id1", "--tab", "123")
	if !strings.Contains(stdout, "deleted tab gid=123") {
		t.Errorf("output = %q, want delete summary", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	assertRequestType(t, got.Body, "deleteSheet")
}

func TestTabsDelete_DuplicateTitleRequiresGID(t *testing.T) {
	dup := `{"spreadsheetId":"id1","properties":{"title":"T"},"sheets":[{"properties":{"sheetId":1,"title":"Dup"}},{"properties":{"sheetId":2,"title":"Dup"}}]}`
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1": {http.StatusOK, dup},
	})
	result, _, stderr := f.run(t, "tabs", "delete", "id1", "--tab", "Dup")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "pass the numeric gid") {
		t.Errorf("stderr = %q, want the ambiguous-title guidance", stderr)
	}
}

func TestTabsDuplicate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1":              {http.StatusOK, metaBody},
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{"duplicateSheet":{"properties":{"sheetId":999,"title":"Data copy"}}}]}`},
	})
	stdout := f.runOK(t, "tabs", "duplicate", "id1", "--tab", "Data", "--title", "Data copy")
	if !strings.Contains(stdout, "new tab gid=999") {
		t.Errorf("output = %q, want new gid", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	req := firstRequest(t, got.Body)
	dup, ok := req["duplicateSheet"].(map[string]any)
	if !ok {
		t.Fatalf("request = %v, want duplicateSheet", req)
	}
	if dup["newSheetName"] != "Data copy" {
		t.Errorf("newSheetName = %v, want \"Data copy\"", dup["newSheetName"])
	}
}

func TestTabsCopyTo(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1":                    {http.StatusOK, metaBody},
		"POST /v4/spreadsheets/id1/sheets/123:copyTo": {http.StatusOK, `{"sheetId":77,"title":"Data"}`},
	})
	stdout := f.runOK(t, "tabs", "copy-to", "id1", "--tab", "Data", "--dest", "https://docs.google.com/spreadsheets/d/dest99/edit")
	if !strings.Contains(stdout, "copied tab gid=123 to spreadsheet dest99") {
		t.Errorf("output = %q, want copy summary with parsed dest id", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1/sheets/123:copyTo")
	var payload struct {
		DestinationSpreadsheetID string `json:"destinationSpreadsheetId"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.DestinationSpreadsheetID != "dest99" {
		t.Errorf("destinationSpreadsheetId = %q, want dest99", payload.DestinationSpreadsheetID)
	}
}

// firstRequest decodes a batchUpdate body and returns its first request object.
func firstRequest(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode batchUpdate body: %v", err)
	}
	if len(payload.Requests) != 1 {
		t.Fatalf("requests = %v, want exactly one", payload.Requests)
	}
	return payload.Requests[0]
}

func assertRequestType(t *testing.T, body []byte, key string) {
	t.Helper()
	req := firstRequest(t, body)
	if _, ok := req[key]; !ok {
		t.Errorf("request = %v, want a %s request", req, key)
	}
}
