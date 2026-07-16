package docs

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleDoc is a documents.get fixture: a title paragraph, a heading, a styled
// paragraph, a bulleted list, and a 2x2 table.
const sampleDoc = `{
  "documentId": "d1",
  "title": "Quarterly Plan",
  "body": {"content": [
    {"endIndex": 15, "paragraph": {"paragraphStyle": {"namedStyleType": "HEADING_1"},
      "elements": [{"textRun": {"content": "Overview\n", "textStyle": {}}}]}},
    {"endIndex": 40, "paragraph": {"paragraphStyle": {"namedStyleType": "NORMAL_TEXT"},
      "elements": [
        {"textRun": {"content": "This is ", "textStyle": {}}},
        {"textRun": {"content": "bold", "textStyle": {"bold": true}}},
        {"textRun": {"content": " and ", "textStyle": {}}},
        {"textRun": {"content": "a link", "textStyle": {"link": {"url": "https://helio.im"}}}},
        {"textRun": {"content": ".\n", "textStyle": {}}}
      ]}},
    {"endIndex": 50, "paragraph": {"bullet": {"listId": "L1", "nestingLevel": 0},
      "elements": [{"textRun": {"content": "first item\n", "textStyle": {}}}]}},
    {"endIndex": 62, "table": {"tableRows": [
      {"tableCells": [
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "H1\n", "textStyle": {}}}]}}]},
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "H2\n", "textStyle": {}}}]}}]}
      ]},
      {"tableCells": [
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "a\n", "textStyle": {}}}]}}]},
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "b\n", "textStyle": {}}}]}}]}
      ]}
    ]}}
  ]},
  "lists": {"L1": {"listProperties": {"nestingLevels": [{"glyphType": "GLYPH_TYPE_UNSPECIFIED"}]}}}
}`

func TestDocumentsGet_MarkdownRendering(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/documents/d1": {http.StatusOK, sampleDoc},
	})
	stdout := f.runOK(t, "documents", "get", "https://docs.google.com/document/d/d1/edit")
	for _, want := range []string{
		"# Quarterly Plan",
		"# Overview",
		"This is **bold** and [a link](https://helio.im).",
		"- first item",
		"| H1 | H2 |",
		"| --- | --- |",
		"| a | b |",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("markdown output missing %q\n---\n%s", want, stdout)
		}
	}
	// URL form must have been reduced to the bare id in the request path.
	f.last(t, "GET", "/v1/documents/d1")
}

func TestDocumentsGet_TextAndJSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/documents/d1": {http.StatusOK, sampleDoc},
	})
	text := f.runOK(t, "documents", "get", "d1", "--format", "text")
	if strings.Contains(text, "**") || strings.Contains(text, "# ") {
		t.Errorf("text output must not carry markdown markers: %q", text)
	}
	if !strings.Contains(text, "This is bold and a link.") {
		t.Errorf("text output missing plain paragraph: %q", text)
	}

	raw := f.runOK(t, "documents", "get", "d1", "--json")
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
	if probe["documentId"] != "d1" {
		t.Errorf("--json output = %q, want the raw provider body", raw)
	}
}

func TestDocumentsGet_QueryParams(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/documents/d1": {http.StatusOK, sampleDoc},
	})
	f.runOK(t, "documents", "get", "d1", "--suggestions", "preview-accept", "--all-tabs")
	got := f.last(t, "GET", "/v1/documents/d1")
	if !strings.Contains(got.Query, "suggestionsViewMode=PREVIEW_SUGGESTIONS_ACCEPTED") {
		t.Errorf("query = %q, want the mapped suggestionsViewMode", got.Query)
	}
	if !strings.Contains(got.Query, "includeTabsContent=true") {
		t.Errorf("query = %q, want includeTabsContent=true", got.Query)
	}
}

func TestDocumentsCreate_Simple(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/documents": {http.StatusOK, `{"documentId":"NEW1","title":"Notes"}`},
	})
	stdout := f.runOK(t, "documents", "create", "--title", "Notes")
	if !strings.Contains(stdout, "NEW1") || !strings.Contains(stdout, "docs.google.com/document/d/NEW1") {
		t.Errorf("output = %q, want id + url", stdout)
	}
	got := f.last(t, "POST", "/v1/documents")
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload["title"] != "Notes" {
		t.Errorf("request title = %v, want Notes", payload["title"])
	}
}

func TestDocumentsCreate_WithBodyFile(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyFile, []byte("# Title\n\nsome **bold** text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /v1/documents":                  {http.StatusOK, `{"documentId":"NEW1","title":"Notes"}`},
		"POST /v1/documents/NEW1:batchUpdate": {http.StatusOK, `{"documentId":"NEW1","replies":[]}`},
	})
	f.runOK(t, "documents", "create", "--title", "Notes", "--body-file", bodyFile)
	got := f.last(t, "POST", "/v1/documents/NEW1:batchUpdate")
	var payload struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("batchUpdate body not JSON: %v", err)
	}
	if len(payload.Requests) == 0 {
		t.Fatal("expected batchUpdate requests for the body markdown")
	}
	insert, ok := payload.Requests[0]["insertText"].(map[string]any)
	if !ok {
		t.Fatalf("first request is not insertText: %v", payload.Requests[0])
	}
	if !strings.Contains(insert["text"].(string), "some bold text") {
		t.Errorf("insertText text = %q, want the stripped markdown", insert["text"])
	}
}

func TestDocumentsCreate_BodyWriteFailureStillReportsURL(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyFile, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"POST /v1/documents":                  {http.StatusOK, `{"documentId":"NEW1","title":"Notes"}`},
		"POST /v1/documents/NEW1:batchUpdate": {http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT","message":"bad request"}}`},
	})
	result, stdout, stderr := f.run(t, "documents", "create", "--title", "Notes", "--body-file", bodyFile)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 on body write failure", result.ExitCode)
	}
	if !strings.Contains(stdout, "docs.google.com/document/d/NEW1") {
		t.Errorf("stdout = %q, want the created document URL despite the failure", stdout)
	}
	if !strings.Contains(stderr, "writing the body failed") {
		t.Errorf("stderr = %q, want the explicit body-write failure", stderr)
	}
}

func TestDocumentsAppend_TextUsesEndOfSegment(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/documents/d1:batchUpdate": {http.StatusOK, `{"documentId":"d1","replies":[]}`},
	})
	f.runOK(t, "documents", "append", "d1", "--text", "one more line")
	if len(f.requests) != 1 {
		t.Fatalf("append --text must be a single batchUpdate, saw %d requests", len(f.requests))
	}
	got := f.last(t, "POST", "/v1/documents/d1:batchUpdate")
	var payload struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	insert := payload.Requests[0]["insertText"].(map[string]any)
	if _, ok := insert["endOfSegmentLocation"]; !ok {
		t.Errorf("append --text must use endOfSegmentLocation, got %v", insert)
	}
	if _, ok := insert["location"]; ok {
		t.Errorf("append --text must not carry an explicit index location: %v", insert)
	}
}

func TestDocumentsAppend_BodyFileReadsThenWritesWithIndex(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyFile, []byte("appended para\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, map[string]route{
		"GET /v1/documents/d1":              {http.StatusOK, sampleDoc},
		"POST /v1/documents/d1:batchUpdate": {http.StatusOK, `{"documentId":"d1","replies":[]}`},
	})
	f.runOK(t, "documents", "append", "d1", "--body-file", bodyFile)
	// First a read to compute the index, then the write.
	if len(f.requests) != 2 || f.requests[0].Method != http.MethodGet {
		t.Fatalf("append --body-file must GET then POST; got %d requests", len(f.requests))
	}
	got := f.last(t, "POST", "/v1/documents/d1:batchUpdate")
	var payload struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	insert := payload.Requests[0]["insertText"].(map[string]any)
	loc, ok := insert["location"].(map[string]any)
	if !ok {
		t.Fatalf("append --body-file must insert at an explicit index: %v", insert)
	}
	// sampleDoc's last endIndex is 62 → insert index 61.
	if loc["index"].(float64) != 61 {
		t.Errorf("insert index = %v, want 61 (segment end - 1)", loc["index"])
	}
}

func TestDocumentsReplaceAll_ReportsOccurrences(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/documents/d1:batchUpdate": {http.StatusOK, `{"documentId":"d1","replies":[{"replaceAllText":{"occurrencesChanged":4}}]}`},
	})
	stdout := f.runOK(t, "documents", "replace-all", "d1", "--find", "foo", "--replace", "bar", "--match-case")
	if !strings.Contains(stdout, "replaced 4 occurrence(s) of \"foo\"") {
		t.Errorf("output = %q, want the occurrencesChanged count", stdout)
	}
	got := f.last(t, "POST", "/v1/documents/d1:batchUpdate")
	var payload struct {
		Requests []struct {
			ReplaceAllText struct {
				ReplaceText  string `json:"replaceText"`
				ContainsText struct {
					Text      string `json:"text"`
					MatchCase bool   `json:"matchCase"`
				} `json:"containsText"`
			} `json:"replaceAllText"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	r := payload.Requests[0].ReplaceAllText
	if r.ReplaceText != "bar" || r.ContainsText.Text != "foo" || !r.ContainsText.MatchCase {
		t.Errorf("replaceAllText = %+v, want find=foo replace=bar matchCase=true", r)
	}
}

func TestDocumentsBatchUpdate_PassthroughForms(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"array form", `[{"insertText":{"text":"x","endOfSegmentLocation":{}}}]`},
		{"envelope form", `{"requests":[{"insertText":{"text":"x","endOfSegmentLocation":{}}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			reqFile := filepath.Join(dir, "req.json")
			if err := os.WriteFile(reqFile, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, map[string]route{
				"POST /v1/documents/d1:batchUpdate": {http.StatusOK, `{"documentId":"d1","replies":[{}]}`},
			})
			stdout := f.runOK(t, "documents", "batch-update", "d1", "--requests-file", reqFile, "--json")
			if !strings.Contains(stdout, `"documentId":"d1"`) {
				t.Errorf("--json output = %q, want raw replies", stdout)
			}
			got := f.last(t, "POST", "/v1/documents/d1:batchUpdate")
			var payload struct {
				Requests []map[string]any `json:"requests"`
			}
			if err := json.Unmarshal(got.Body, &payload); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			if len(payload.Requests) != 1 {
				t.Errorf("requests = %v, want the single passthrough insertText", payload.Requests)
			}
		})
	}
}
