package slides

import "testing"

func TestExtractPresentationID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1a2b3c", "1a2b3c"},
		{"https://docs.google.com/presentation/d/1a2b3c/edit", "1a2b3c"},
		{"https://docs.google.com/presentation/d/1a2b3c/edit#slide=id.p1", "1a2b3c"},
		{"https://docs.google.com/presentation/d/1a2b3c", "1a2b3c"},
		{"https://docs.google.com/presentation/d/1a2b3c/edit?usp=sharing", "1a2b3c"},
		{"  1a2b3c  ", "1a2b3c"},
	}
	for _, tc := range cases {
		if got := extractPresentationID(tc.in); got != tc.want {
			t.Errorf("extractPresentationID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseTextRange(t *testing.T) {
	cases := []struct {
		spec      string
		wantType  string
		wantStart any
		wantEnd   any
		wantErr   bool
	}{
		{"", "ALL", nil, nil, false},
		{"2:5", "FIXED_RANGE", 2, 5, false},
		{"3:", "FROM_START_INDEX", 3, nil, false},
		{"5:2", "", nil, nil, true},
		{"nope", "", nil, nil, true},
		{"-1:2", "", nil, nil, true},
	}
	for _, tc := range cases {
		got, err := parseTextRange(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTextRange(%q) expected error", tc.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTextRange(%q) unexpected error: %v", tc.spec, err)
			continue
		}
		if got["type"] != tc.wantType {
			t.Errorf("parseTextRange(%q) type = %v, want %v", tc.spec, got["type"], tc.wantType)
		}
		if tc.wantStart != nil && got["startIndex"] != tc.wantStart {
			t.Errorf("parseTextRange(%q) start = %v, want %v", tc.spec, got["startIndex"], tc.wantStart)
		}
		if tc.wantEnd != nil && got["endIndex"] != tc.wantEnd {
			t.Errorf("parseTextRange(%q) end = %v, want %v", tc.spec, got["endIndex"], tc.wantEnd)
		}
	}
}

func TestParseSize(t *testing.T) {
	got, err := parseSize("400x300")
	if err != nil {
		t.Fatalf("parseSize error: %v", err)
	}
	w := got["width"].(map[string]any)
	if w["magnitude"].(float64) != 400 || w["unit"] != "PT" {
		t.Errorf("width = %v, want 400 PT", w)
	}
	if _, err := parseSize("400"); err == nil {
		t.Error("parseSize(\"400\") expected error")
	}
	if _, err := parseSize("0x300"); err == nil {
		t.Error("parseSize(\"0x300\") expected error (non-positive width)")
	}
}

func TestParseTransform(t *testing.T) {
	got, err := parseTransform("100,50")
	if err != nil {
		t.Fatalf("parseTransform error: %v", err)
	}
	if got["translateX"].(float64) != 100 || got["translateY"].(float64) != 50 {
		t.Errorf("transform = %v, want translate 100,50", got)
	}
	if got["scaleX"] != 1 || got["scaleY"] != 1 {
		t.Errorf("transform scale = %v/%v, want 1/1", got["scaleX"], got["scaleY"])
	}
	if _, err := parseTransform("100"); err == nil {
		t.Error("parseTransform(\"100\") expected error")
	}
}

func TestNormalizeBatchRequests(t *testing.T) {
	if _, err := normalizeBatchRequests([]byte("")); err == nil {
		t.Error("empty input should error")
	}
	if _, err := normalizeBatchRequests([]byte("not json")); err == nil {
		t.Error("invalid JSON should error")
	}
	if _, err := normalizeBatchRequests([]byte(`"a string"`)); err == nil {
		t.Error("a bare JSON string is neither array nor object; should error")
	}
	if _, err := normalizeBatchRequests([]byte(`[{"deleteObject":{"objectId":"a"}}]`)); err != nil {
		t.Errorf("array input should succeed: %v", err)
	}
}

func TestUTF16Len(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"point one\npoint two\n", 20},
		{"café", 4},
		{"😀", 2}, // astral plane -> surrogate pair -> 2 UTF-16 units
	}
	for _, tc := range cases {
		if got := utf16Len(tc.in); got != tc.want {
			t.Errorf("utf16Len(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
