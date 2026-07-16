package sheets

import (
	"strings"
	"testing"
)

func TestParseSpreadsheetID(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"bare id", "1AbC_dEfGhIjK", "1AbC_dEfGhIjK", false},
		{"full url with edit and gid", "https://docs.google.com/spreadsheets/d/1AbC_dEfGhIjK/edit#gid=0", "1AbC_dEfGhIjK", false},
		{"url without trailing path", "https://docs.google.com/spreadsheets/d/1AbC_dEfGhIjK", "1AbC_dEfGhIjK", false},
		{"url with query", "https://docs.google.com/spreadsheets/d/1AbC_dEfGhIjK/edit?usp=sharing", "1AbC_dEfGhIjK", false},
		{"whitespace trimmed", "  1AbC_dEfGhIjK  ", "1AbC_dEfGhIjK", false},
		{"empty", "", "", true},
		{"url without /d/", "https://docs.google.com/spreadsheets/foo", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSpreadsheetID(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseSpreadsheetID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveTabID(t *testing.T) {
	meta := &spreadsheetMeta{}
	meta.Sheets = []struct {
		Properties sheetProps `json:"properties"`
	}{
		{Properties: sheetProps{SheetID: 0, Title: "Sheet1"}},
		{Properties: sheetProps{SheetID: 123, Title: "Q3 Budget"}},
		{Properties: sheetProps{SheetID: 456, Title: "Dup"}},
		{Properties: sheetProps{SheetID: 789, Title: "Dup"}},
	}
	cases := []struct {
		name    string
		tab     string
		want    int64
		wantErr string
	}{
		{"by title", "Q3 Budget", 123, ""},
		{"by title zero gid", "Sheet1", 0, ""},
		{"by gid", "123", 123, ""},
		{"by gid zero", "0", 0, ""},
		{"duplicate title requires gid", "Dup", 0, "pass the numeric gid"},
		{"unknown title", "Nope", 0, "no tab named"},
		{"unknown gid", "9999", 0, "no tab with gid"},
		{"empty", "", 0, "--tab is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTabID(meta, tc.tab)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveTabID(%q) = %d, want %d", tc.tab, got, tc.want)
			}
		})
	}
}
