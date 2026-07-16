package docs

import (
	"testing"
)

func TestParseInline_Styles(t *testing.T) {
	text, runs := parseInline("plain **bold** and *italic* then ~~gone~~ and `code` and [label](https://x.io) end")
	want := "plain bold and italic then gone and code and label end"
	if text != want {
		t.Fatalf("stripped text = %q, want %q", text, want)
	}
	// Verify each run maps to the right substring in the stripped text.
	byFlag := map[string]inlineRun{}
	for _, r := range runs {
		switch {
		case r.bold:
			byFlag["bold"] = r
		case r.italic:
			byFlag["italic"] = r
		case r.strike:
			byFlag["strike"] = r
		case r.code:
			byFlag["code"] = r
		case r.link != "":
			byFlag["link"] = r
		}
	}
	check := func(flag, sub string) {
		r, ok := byFlag[flag]
		if !ok {
			t.Errorf("missing %s run", flag)
			return
		}
		if got := text[r.start:r.end]; got != sub {
			t.Errorf("%s run covers %q, want %q", flag, got, sub)
		}
	}
	check("bold", "bold")
	check("italic", "italic")
	check("strike", "gone")
	check("code", "code")
	check("link", "label")
	if byFlag["link"].link != "https://x.io" {
		t.Errorf("link url = %q, want https://x.io", byFlag["link"].link)
	}
}

func TestParseInline_UnclosedMarkerIsLiteral(t *testing.T) {
	text, runs := parseInline("a *dangling star")
	if text != "a *dangling star" {
		t.Errorf("text = %q, want the literal asterisk preserved", text)
	}
	if len(runs) != 0 {
		t.Errorf("runs = %v, want none for an unclosed marker", runs)
	}
}

func TestParseInline_UTF16Offsets(t *testing.T) {
	// An astral emoji is two UTF-16 code units; offsets must count in UTF-16.
	text, runs := parseInline("😀 **b**")
	if text != "😀 b" {
		t.Fatalf("text = %q", text)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want one bold run", runs)
	}
	// "😀 " is 2 (emoji) + 1 (space) = 3 UTF-16 units.
	if runs[0].start != 3 {
		t.Errorf("bold run start = %d, want 3 UTF-16 units", runs[0].start)
	}
}

func TestMarkdownToRequests_Structure(t *testing.T) {
	md := "# Heading\n\nbody **x**\n- item one\n1. item two\n"
	reqs, warnings := markdownToRequests(md, 1, "", "")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if len(reqs) == 0 {
		t.Fatal("no requests produced")
	}
	// First request inserts all text at index 1.
	insert, ok := reqs[0]["insertText"].(map[string]any)
	if !ok {
		t.Fatalf("first request is not insertText: %v", reqs[0])
	}
	if insert["location"].(map[string]any)["index"] != 1 {
		t.Errorf("insert index = %v, want 1", insert["location"])
	}
	kinds := map[string]int{}
	for _, r := range reqs {
		for k := range r {
			kinds[k]++
		}
	}
	if kinds["updateParagraphStyle"] != 1 {
		t.Errorf("updateParagraphStyle count = %d, want 1 (the heading)", kinds["updateParagraphStyle"])
	}
	if kinds["updateTextStyle"] != 1 {
		t.Errorf("updateTextStyle count = %d, want 1 (the bold run)", kinds["updateTextStyle"])
	}
	if kinds["createParagraphBullets"] != 2 {
		t.Errorf("createParagraphBullets count = %d, want 2 (one per list item)", kinds["createParagraphBullets"])
	}
}

func TestMarkdownToRequests_TableWarns(t *testing.T) {
	_, warnings := markdownToRequests("| a | b |\n| --- | --- |\n", 1, "", "")
	if len(warnings) == 0 {
		t.Fatal("expected a degradation warning for a markdown table")
	}
}

func TestMarkdownToRequests_TabScopesLocations(t *testing.T) {
	reqs, _ := markdownToRequests("# Head\n", 5, "\n", "t.7")
	insert := reqs[0]["insertText"].(map[string]any)
	loc := insert["location"].(map[string]any)
	if loc["tabId"] != "t.7" {
		t.Errorf("insert location tabId = %v, want t.7", loc["tabId"])
	}
	if loc["index"] != 5 {
		t.Errorf("insert index = %v, want 5", loc["index"])
	}
	// The heading's paragraph range must also be tab-scoped and offset past the
	// leading newline (base = 5 + len("\n") = 6).
	for _, r := range reqs {
		if ups, ok := r["updateParagraphStyle"].(map[string]any); ok {
			rng := ups["range"].(map[string]any)
			if rng["tabId"] != "t.7" {
				t.Errorf("paragraph range tabId = %v, want t.7", rng["tabId"])
			}
			if rng["startIndex"] != 6 {
				t.Errorf("paragraph start = %v, want 6", rng["startIndex"])
			}
		}
	}
}
