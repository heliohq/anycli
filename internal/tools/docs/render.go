package docs

import (
	"fmt"
	"strings"
)

// The subset of the Docs API Document schema needed to render markdown/text.
// Only the fields that carry visible content and inline styling are decoded;
// everything else (indices, object properties, styling we do not project) is
// intentionally dropped — the raw structure is always available via --format
// json.

type apiDocument struct {
	DocumentID string             `json:"documentId"`
	Title      string             `json:"title"`
	Body       apiBody            `json:"body"`
	Tabs       []apiTab           `json:"tabs"`
	Lists      map[string]apiList `json:"lists"`
}

type apiBody struct {
	Content []apiStructuralElement `json:"content"`
}

type apiTab struct {
	TabProperties apiTabProperties `json:"tabProperties"`
	DocumentTab   *apiDocumentTab  `json:"documentTab"`
	ChildTabs     []apiTab         `json:"childTabs"`
}

type apiTabProperties struct {
	TabID string `json:"tabId"`
	Title string `json:"title"`
}

type apiDocumentTab struct {
	Body  apiBody            `json:"body"`
	Lists map[string]apiList `json:"lists"`
}

type apiStructuralElement struct {
	EndIndex  int           `json:"endIndex"`
	Paragraph *apiParagraph `json:"paragraph"`
	Table     *apiTable     `json:"table"`
}

type apiParagraph struct {
	Elements       []apiParagraphElement `json:"elements"`
	ParagraphStyle apiParagraphStyle     `json:"paragraphStyle"`
	Bullet         *apiBullet            `json:"bullet"`
}

type apiParagraphStyle struct {
	NamedStyleType string `json:"namedStyleType"`
}

type apiBullet struct {
	ListID       string `json:"listId"`
	NestingLevel int    `json:"nestingLevel"`
}

type apiParagraphElement struct {
	TextRun *apiTextRun `json:"textRun"`
}

type apiTextRun struct {
	Content   string       `json:"content"`
	TextStyle apiTextStyle `json:"textStyle"`
}

type apiTextStyle struct {
	Bold          bool     `json:"bold"`
	Italic        bool     `json:"italic"`
	Strikethrough bool     `json:"strikethrough"`
	Link          *apiLink `json:"link"`
}

type apiLink struct {
	URL string `json:"url"`
}

type apiTable struct {
	TableRows []apiTableRow `json:"tableRows"`
}

type apiTableRow struct {
	TableCells []apiTableCell `json:"tableCells"`
}

type apiTableCell struct {
	Content []apiStructuralElement `json:"content"`
}

type apiList struct {
	ListProperties apiListProperties `json:"listProperties"`
}

type apiListProperties struct {
	NestingLevels []apiNestingLevel `json:"nestingLevels"`
}

type apiNestingLevel struct {
	GlyphType string `json:"glyphType"`
}

// headingPrefix maps a Docs named paragraph style to its markdown heading
// prefix. TITLE and HEADING_1 both render as a single '#'.
func headingPrefix(namedStyleType string) string {
	switch namedStyleType {
	case "TITLE", "HEADING_1":
		return "# "
	case "HEADING_2":
		return "## "
	case "HEADING_3":
		return "### "
	case "HEADING_4":
		return "#### "
	case "HEADING_5":
		return "##### "
	case "HEADING_6":
		return "###### "
	default:
		return ""
	}
}

// orderedGlyphTypes are the glyph types that denote an ordered (numbered) list.
var orderedGlyphTypes = map[string]bool{
	"DECIMAL": true, "ZERO_DECIMAL": true, "UPPER_ALPHA": true,
	"ALPHA": true, "UPPER_ROMAN": true, "ROMAN": true,
}

// renderDocument renders a whole document to markdown or plain text. When the
// document has tabs (includeTabsContent), only the tabs matching tabFilter are
// rendered (empty filter = all tabs).
func renderDocument(doc apiDocument, asMarkdown bool, tabFilter string) string {
	var b strings.Builder
	if asMarkdown && doc.Title != "" {
		fmt.Fprintf(&b, "# %s\n\n", doc.Title)
	}
	if len(doc.Tabs) > 0 {
		for _, tab := range doc.Tabs {
			renderTab(&b, tab, asMarkdown, tabFilter)
		}
		return b.String()
	}
	renderBody(&b, doc.Body, doc.Lists, asMarkdown)
	return b.String()
}

// renderTab renders one tab (and its children) when it passes the filter.
func renderTab(b *strings.Builder, tab apiTab, asMarkdown bool, tabFilter string) {
	if tabFilter == "" || tab.TabProperties.TabID == tabFilter {
		if asMarkdown && tab.TabProperties.Title != "" {
			fmt.Fprintf(b, "\n## %s\n\n", tab.TabProperties.Title)
		}
		if tab.DocumentTab != nil {
			renderBody(b, tab.DocumentTab.Body, tab.DocumentTab.Lists, asMarkdown)
		}
	}
	for _, child := range tab.ChildTabs {
		renderTab(b, child, asMarkdown, tabFilter)
	}
}

// renderBody renders a body's structural elements.
func renderBody(b *strings.Builder, body apiBody, lists map[string]apiList, asMarkdown bool) {
	for _, el := range body.Content {
		switch {
		case el.Paragraph != nil:
			b.WriteString(renderParagraph(*el.Paragraph, lists, asMarkdown))
		case el.Table != nil && asMarkdown:
			b.WriteString(renderTable(*el.Table, lists))
		case el.Table != nil:
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					renderBody(b, apiBody{Content: cell.Content}, lists, asMarkdown)
				}
			}
		}
	}
}

// renderParagraph renders one paragraph as a single markdown/text line.
func renderParagraph(p apiParagraph, lists map[string]apiList, asMarkdown bool) string {
	text := inlineText(p.Elements, asMarkdown)
	text = strings.TrimRight(text, "\n")
	if !asMarkdown {
		return text + "\n"
	}
	if p.Bullet != nil {
		indent := strings.Repeat("  ", p.Bullet.NestingLevel)
		marker := "- "
		if isOrderedList(p.Bullet, lists) {
			marker = "1. "
		}
		return indent + marker + text + "\n"
	}
	return headingPrefix(p.ParagraphStyle.NamedStyleType) + text + "\n"
}

// isOrderedList reports whether the paragraph's list uses a numbered glyph.
func isOrderedList(bullet *apiBullet, lists map[string]apiList) bool {
	list, ok := lists[bullet.ListID]
	if !ok {
		return false
	}
	if bullet.NestingLevel < len(list.ListProperties.NestingLevels) {
		return orderedGlyphTypes[list.ListProperties.NestingLevels[bullet.NestingLevel].GlyphType]
	}
	return false
}

// inlineText concatenates a paragraph's text runs, applying markdown emphasis
// when asMarkdown is set.
func inlineText(elements []apiParagraphElement, asMarkdown bool) string {
	var b strings.Builder
	for _, el := range elements {
		if el.TextRun == nil {
			continue
		}
		content := el.TextRun.Content
		if !asMarkdown {
			b.WriteString(content)
			continue
		}
		b.WriteString(styleRun(content, el.TextRun.TextStyle))
	}
	return b.String()
}

// styleRun wraps a run's text in markdown emphasis, preserving any trailing
// newline outside the emphasis markers so line breaks are not swallowed.
func styleRun(content string, style apiTextStyle) string {
	trimmed := strings.TrimRight(content, "\n")
	trailing := content[len(trimmed):]
	if strings.TrimSpace(trimmed) == "" {
		return content
	}
	out := trimmed
	if style.Strikethrough {
		out = "~~" + out + "~~"
	}
	if style.Bold {
		out = "**" + out + "**"
	}
	if style.Italic {
		out = "*" + out + "*"
	}
	if style.Link != nil && style.Link.URL != "" {
		out = "[" + out + "](" + style.Link.URL + ")"
	}
	return out + trailing
}

// renderTable renders a table as a GitHub-flavored markdown table, using the
// first row as the header.
func renderTable(t apiTable, lists map[string]apiList) string {
	if len(t.TableRows) == 0 {
		return ""
	}
	var b strings.Builder
	cols := 0
	for _, row := range t.TableRows {
		if len(row.TableCells) > cols {
			cols = len(row.TableCells)
		}
	}
	for i, row := range t.TableRows {
		b.WriteString("|")
		for c := 0; c < cols; c++ {
			cellText := ""
			if c < len(row.TableCells) {
				cellText = cellPlain(row.TableCells[c], lists)
			}
			fmt.Fprintf(&b, " %s |", cellText)
		}
		b.WriteString("\n")
		if i == 0 {
			b.WriteString("|")
			for c := 0; c < cols; c++ {
				b.WriteString(" --- |")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// cellPlain renders a table cell to a single markdown-safe line: emphasis is
// kept, but newlines become spaces and pipes are escaped so the row stays well
// formed.
func cellPlain(cell apiTableCell, lists map[string]apiList) string {
	var b strings.Builder
	renderBody(&b, apiBody{Content: cell.Content}, lists, true)
	text := strings.TrimSpace(b.String())
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "|", "\\|")
	return strings.Join(strings.Fields(text), " ")
}
