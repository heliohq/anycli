package sheets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpreadsheetsGet_HumanAndJSON(t *testing.T) {
	body := `{"spreadsheetId":"id1","spreadsheetUrl":"https://docs.google.com/spreadsheets/d/id1/edit","properties":{"title":"Budget"},"sheets":[{"properties":{"sheetId":0,"title":"Q3","gridProperties":{"rowCount":1000,"columnCount":26}}}]}`
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "spreadsheets", "get", "id1")
	if !strings.Contains(stdout, "Budget") || !strings.Contains(stdout, "Q3") || !strings.Contains(stdout, "gid=0") || !strings.Contains(stdout, "1000x26") {
		t.Errorf("human output = %q, want title + tab + scale", stdout)
	}
	got := f.last(t, "GET", "/v4/spreadsheets/id1")
	if !strings.Contains(got.Query, "fields=") {
		t.Errorf("query = %q, want a fields mask (no grid data)", got.Query)
	}

	stdout = f.runOK(t, "spreadsheets", "get", "id1", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want raw provider body", stdout)
	}
}

func TestSpreadsheetsGet_AcceptsURL(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v4/spreadsheets/id1": {http.StatusOK, `{"spreadsheetId":"id1","properties":{"title":"T"},"sheets":[]}`},
	})
	f.runOK(t, "spreadsheets", "get", "https://docs.google.com/spreadsheets/d/id1/edit#gid=0")
	// The fixture only routes /v4/spreadsheets/id1, so a successful call proves
	// the URL was parsed down to the bare id.
	f.last(t, "GET", "/v4/spreadsheets/id1")
}

func TestSpreadsheetsCreate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets": {http.StatusOK, `{"spreadsheetId":"newid","spreadsheetUrl":"https://docs.google.com/spreadsheets/d/newid/edit"}`},
	})
	stdout := f.runOK(t, "spreadsheets", "create", "--title", "Plan", "--tab", "Jan", "--tab", "Feb")
	if !strings.Contains(stdout, "newid") || !strings.Contains(stdout, "Plan") {
		t.Errorf("output = %q, want new id + title", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets")
	var payload struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Properties.Title != "Plan" || len(payload.Sheets) != 2 {
		t.Errorf("body = %+v, want title Plan and two tabs", payload)
	}
}

func TestSpreadsheetsBatchUpdate_ArrayFile(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "req.json")
	if err := os.WriteFile(reqPath, []byte(`[{"repeatCell":{"range":{"sheetId":0}}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{}]}`},
	})
	stdout := f.runOK(t, "spreadsheets", "batch-update", "id1", "--request-file", reqPath)
	if !strings.Contains(stdout, "batch update applied (1 replies)") {
		t.Errorf("output = %q, want reply count", stdout)
	}
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	var payload struct {
		Requests []json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload.Requests) != 1 {
		t.Errorf("body = %s, want a bare array wrapped into requests[]", got.Body)
	}
}

func TestSpreadsheetsBatchUpdate_ObjectFile(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "req.json")
	if err := os.WriteFile(reqPath, []byte(`{"requests":[{"deleteSheet":{"sheetId":9}}],"includeSpreadsheetInResponse":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /v4/spreadsheets/id1:batchUpdate": {http.StatusOK, `{"replies":[{}]}`},
	})
	f.runOK(t, "spreadsheets", "batch-update", "id1", "--request-file", reqPath)
	got := f.last(t, "POST", "/v4/spreadsheets/id1:batchUpdate")
	if !strings.Contains(string(got.Body), "includeSpreadsheetInResponse") {
		t.Errorf("body = %s, want the full object passed through verbatim", got.Body)
	}
}
