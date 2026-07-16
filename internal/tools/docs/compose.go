package docs

import (
	"regexp"
	"strings"
	"unicode/utf16"
)

// The write direction: translate a markdown subset into a batchUpdate request
// sequence so the caller never computes a UTF-16 index. The supported subset is
// headings, paragraphs, bold / italic / strikethrough / inline code, links, and
// ordered / unordered lists. Tables and images are NOT written (they are the
// index-arithmetic and public-URI danger zones — the --requests-file escape
// hatch covers them); markdown for them degrades to literal text with a
// warning. See design 303 §markdown 写方向是子集.

// blockKind classifies a parsed markdown line.
type blockKind int

const (
	blockParagraph blockKind = iota
	blockHeading
	blockUnordered
	blockOrdered
)

// mdBlock is one parsed markdown line: its plain text plus the inline styling
// ranges within that text (offsets in UTF-16 code units).
type mdBlock struct {
	text  string
	runs  []inlineRun
	kind  blockKind
	level int // heading level 1..6, or list nesting level
}

// inlineRun is a styled range within a block's plain text. start/end are
// UTF-16 code-unit offsets.
type inlineRun struct {
	start, end           int
	bold, italic, strike bool
	code                 bool
	link                 string
}

var (
	headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	orderedRe   = regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)
	unorderedRe = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
)

// u16len returns the length of s in UTF-16 code units, the unit Docs indices
// use.
func u16len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// markdownToRequests parses markdown and builds the batchUpdate request
// sequence that inserts it. Text is inserted at insertIndex (prefixed by
// prefix, e.g. a leading newline for append); every styling request carries an
// explicit index computed from insertIndex, so requests apply in-order against
// a known base. tabID, when non-empty, scopes every location and range to that
// document tab. Returns the requests plus any degradation warnings.
func markdownToRequests(md string, insertIndex int, prefix, tabID string) ([]map[string]any, []string) {
	blocks, warnings := parseBlocks(md)
	if len(blocks) == 0 {
		return nil, warnings
	}

	var combined strings.Builder
	combined.WriteString(prefix)

	// blockStart[i] is the absolute UTF-16 index where block i's text begins.
	base := insertIndex + u16len(prefix)
	blockStart := make([]int, len(blocks))
	cursor := base
	for i, blk := range blocks {
		blockStart[i] = cursor
		combined.WriteString(blk.text)
		combined.WriteString("\n")
		cursor += u16len(blk.text) + 1
	}

	location := map[string]any{"index": insertIndex}
	if tabID != "" {
		location["tabId"] = tabID
	}
	reqs := []map[string]any{
		{"insertText": map[string]any{
			"text":     combined.String(),
			"location": location,
		}},
	}

	// Paragraph styles (headings) — do not shift indices.
	for i, blk := range blocks {
		if blk.kind != blockHeading {
			continue
		}
		start, end := blockStart[i], blockStart[i]+u16len(blk.text)+1
		reqs = append(reqs, map[string]any{"updateParagraphStyle": map[string]any{
			"range":          rangeMap(start, end, tabID),
			"paragraphStyle": map[string]any{"namedStyleType": headingStyleName(blk.level)},
			"fields":         "namedStyleType",
		}})
	}

	// Inline text styles — do not shift indices.
	for i, blk := range blocks {
		for _, run := range blk.runs {
			reqs = append(reqs, textStyleRequest(blockStart[i]+run.start, blockStart[i]+run.end, tabID, run))
		}
	}

	// Bullets last, in reverse document order: createParagraphBullets can shift
	// indices, so applying later ranges first keeps earlier ranges valid.
	//
	// List nesting is intentionally flattened: the Docs API derives nesting from
	// leading tab characters that createParagraphBullets counts then strips,
	// which shifts every downstream index — the index-arithmetic danger zone the
	// write subset avoids (design 303 §markdown 写方向是子集). blk.level is parsed
	// for lists but deliberately not applied here; nested lists need
	// batch-update --requests-file. Reads still render nesting via indent.
	for i := len(blocks) - 1; i >= 0; i-- {
		blk := blocks[i]
		if blk.kind != blockUnordered && blk.kind != blockOrdered {
			continue
		}
		start, end := blockStart[i], blockStart[i]+u16len(blk.text)+1
		preset := "BULLET_DISC_CIRCLE_SQUARE"
		if blk.kind == blockOrdered {
			preset = "NUMBERED_DECIMAL_ALPHA_ROMAN"
		}
		reqs = append(reqs, map[string]any{"createParagraphBullets": map[string]any{
			"range":        rangeMap(start, end, tabID),
			"bulletPreset": preset,
		}})
	}

	return reqs, warnings
}

func rangeMap(start, end int, tabID string) map[string]any {
	r := map[string]any{"startIndex": start, "endIndex": end}
	if tabID != "" {
		r["tabId"] = tabID
	}
	return r
}

func headingStyleName(level int) string {
	switch level {
	case 1:
		return "HEADING_1"
	case 2:
		return "HEADING_2"
	case 3:
		return "HEADING_3"
	case 4:
		return "HEADING_4"
	case 5:
		return "HEADING_5"
	default:
		return "HEADING_6"
	}
}

// textStyleRequest builds one updateTextStyle covering a run, combining every
// applicable style field into a single request.
func textStyleRequest(start, end int, tabID string, run inlineRun) map[string]any {
	style := map[string]any{}
	var fields []string
	if run.bold {
		style["bold"] = true
		fields = append(fields, "bold")
	}
	if run.italic {
		style["italic"] = true
		fields = append(fields, "italic")
	}
	if run.strike {
		style["strikethrough"] = true
		fields = append(fields, "strikethrough")
	}
	if run.code {
		style["weightedFontFamily"] = map[string]any{"fontFamily": "Consolas"}
		fields = append(fields, "weightedFontFamily")
	}
	if run.link != "" {
		style["link"] = map[string]any{"url": run.link}
		fields = append(fields, "link")
	}
	return map[string]any{"updateTextStyle": map[string]any{
		"range":     rangeMap(start, end, tabID),
		"textStyle": style,
		"fields":    strings.Join(fields, ","),
	}}
}

// parseBlocks splits markdown into line blocks and collects degradation
// warnings for unsupported syntax (tables, images).
func parseBlocks(md string) ([]mdBlock, []string) {
	var blocks []mdBlock
	warned := map[string]bool{}
	var warnings []string
	warn := func(msg string) {
		if !warned[msg] {
			warned[msg] = true
			warnings = append(warnings, msg)
		}
	}

	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "|") {
			warn("markdown tables are not written by v1 — the row was inserted as literal text; use batch-update with --requests-file for real tables")
		}
		if strings.Contains(trimmed, "![") {
			warn("markdown images are not written by v1 — the image markup was inserted as literal text; use batch-update with --requests-file for inline images")
		}

		if m := headingRe.FindStringSubmatch(line); m != nil {
			text, runs := parseInline(m[2])
			blocks = append(blocks, mdBlock{text: text, runs: runs, kind: blockHeading, level: len(m[1])})
			continue
		}
		if m := orderedRe.FindStringSubmatch(line); m != nil {
			text, runs := parseInline(m[2])
			blocks = append(blocks, mdBlock{text: text, runs: runs, kind: blockOrdered, level: len(m[1]) / 2})
			continue
		}
		if m := unorderedRe.FindStringSubmatch(line); m != nil {
			text, runs := parseInline(m[2])
			blocks = append(blocks, mdBlock{text: text, runs: runs, kind: blockUnordered, level: len(m[1]) / 2})
			continue
		}
		text, runs := parseInline(line)
		blocks = append(blocks, mdBlock{text: text, runs: runs, kind: blockParagraph})
	}
	return blocks, warnings
}

// parseInline strips markdown emphasis / links from a line and returns the
// plain text plus the styled ranges (UTF-16 offsets). Emphasis is not nested:
// the inner text of a marker is taken literally. Unclosed markers are emitted
// as literal characters.
func parseInline(s string) (string, []inlineRun) {
	runes := []rune(s)
	var out strings.Builder
	var runs []inlineRun
	pos16 := 0 // UTF-16 length of text written to out so far

	appendPlain := func(text string) {
		out.WriteString(text)
		pos16 += u16len(text)
	}
	appendStyled := func(text string, mut func(*inlineRun)) {
		start := pos16
		out.WriteString(text)
		pos16 += u16len(text)
		r := inlineRun{start: start, end: pos16}
		mut(&r)
		runs = append(runs, r)
	}

	i := 0
	for i < len(runes) {
		// Link: [text](url)
		if runes[i] == '[' {
			if text, url, next, ok := matchLink(runes, i); ok {
				appendStyled(text, func(r *inlineRun) { r.link = url })
				i = next
				continue
			}
		}
		// Bold: **text**
		if hasMarker(runes, i, "**") {
			if inner, next, ok := matchDelim(runes, i, "**"); ok {
				appendStyled(inner, func(r *inlineRun) { r.bold = true })
				i = next
				continue
			}
		}
		// Strikethrough: ~~text~~
		if hasMarker(runes, i, "~~") {
			if inner, next, ok := matchDelim(runes, i, "~~"); ok {
				appendStyled(inner, func(r *inlineRun) { r.strike = true })
				i = next
				continue
			}
		}
		// Inline code: `text`
		if runes[i] == '`' {
			if inner, next, ok := matchDelim(runes, i, "`"); ok {
				appendStyled(inner, func(r *inlineRun) { r.code = true })
				i = next
				continue
			}
		}
		// Italic: *text*
		if runes[i] == '*' {
			if inner, next, ok := matchDelim(runes, i, "*"); ok {
				appendStyled(inner, func(r *inlineRun) { r.italic = true })
				i = next
				continue
			}
		}
		appendPlain(string(runes[i]))
		i++
	}
	return out.String(), runs
}

// hasMarker reports whether runes at i start with marker.
func hasMarker(runes []rune, i int, marker string) bool {
	m := []rune(marker)
	if i+len(m) > len(runes) {
		return false
	}
	for k, r := range m {
		if runes[i+k] != r {
			return false
		}
	}
	return true
}

// matchDelim matches marker...marker starting at i, returning the inner text
// and the index just past the closing marker. The inner text must be non-empty.
func matchDelim(runes []rune, i int, marker string) (string, int, bool) {
	m := []rune(marker)
	start := i + len(m)
	for j := start; j+len(m) <= len(runes); j++ {
		if hasMarker(runes, j, marker) && j > start {
			return string(runes[start:j]), j + len(m), true
		}
	}
	return "", 0, false
}

// matchLink matches [text](url) starting at i (runes[i] == '['), returning the
// display text, url, and the index just past the closing paren.
func matchLink(runes []rune, i int) (string, string, int, bool) {
	closeBracket := -1
	for j := i + 1; j < len(runes); j++ {
		if runes[j] == ']' {
			closeBracket = j
			break
		}
	}
	if closeBracket < 0 || closeBracket+1 >= len(runes) || runes[closeBracket+1] != '(' {
		return "", "", 0, false
	}
	closeParen := -1
	for j := closeBracket + 2; j < len(runes); j++ {
		if runes[j] == ')' {
			closeParen = j
			break
		}
	}
	if closeParen < 0 {
		return "", "", 0, false
	}
	text := string(runes[i+1 : closeBracket])
	url := string(runes[closeBracket+2 : closeParen])
	if text == "" || url == "" {
		return "", "", 0, false
	}
	return text, url, closeParen + 1, true
}
