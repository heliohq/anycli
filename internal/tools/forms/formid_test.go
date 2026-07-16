package forms

import (
	"strings"
	"testing"
)

func TestExtractFormID(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"bare id", "1AbcXYZ_-", "1AbcXYZ_-", ""},
		{"edit url", "https://docs.google.com/forms/d/1AbcXYZ/edit", "1AbcXYZ", ""},
		{"edit url with query", "https://docs.google.com/forms/d/1AbcXYZ/edit?usp=sharing", "1AbcXYZ", ""},
		{"prefill/viewform edit path", "https://docs.google.com/forms/d/1AbcXYZ/viewform", "1AbcXYZ", ""},
		{"responder link rejected", "https://docs.google.com/forms/d/e/1FAIpQL/viewform", "", "responder link"},
		{"empty", "   ", "", "empty form id"},
		{"unknown url", "https://example.com/whatever", "", "not a Forms edit link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractFormID(tc.in)
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
				t.Errorf("extractFormID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
