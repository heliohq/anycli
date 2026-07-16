package slides

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// The Slides Presentation JSON is a machine shape (EMU coordinates + nested
// textElements). These structs decode only what the outline view and the
// synthetic verbs need; unknown fields are ignored.

type presentation struct {
	PresentationID string   `json:"presentationId"`
	Title          string   `json:"title"`
	Slides         []page   `json:"slides"`
	Layouts        []layout `json:"layouts"`
}

type layout struct {
	ObjectID         string `json:"objectId"`
	LayoutProperties struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"layoutProperties"`
}

type page struct {
	ObjectID        string           `json:"objectId"`
	PageElements    []pageElement    `json:"pageElements"`
	SlideProperties *slideProperties `json:"slideProperties"`
	NotesProperties *struct {
		SpeakerNotesObjectID string `json:"speakerNotesObjectId"`
	} `json:"notesProperties"`
}

type slideProperties struct {
	LayoutObjectID string `json:"layoutObjectId"`
	NotesPage      *page  `json:"notesPage"`
}

type pageElement struct {
	ObjectID     string        `json:"objectId"`
	Shape        *shape        `json:"shape"`
	ElementGroup *elementGroup `json:"elementGroup"`
	Table        *table        `json:"table"`
}

type elementGroup struct {
	Children []pageElement `json:"children"`
}

type shape struct {
	ShapeType   string `json:"shapeType"`
	Placeholder *struct {
		Type string `json:"type"`
	} `json:"placeholder"`
	Text *textContent `json:"text"`
}

type table struct {
	TableRows []struct {
		TableCells []struct {
			Text *textContent `json:"text"`
		} `json:"tableCells"`
	} `json:"tableRows"`
}

type textContent struct {
	TextElements []struct {
		TextRun *struct {
			Content string `json:"content"`
		} `json:"textRun"`
	} `json:"textElements"`
}

// shapeText concatenates every textRun content in a text body.
func shapeText(tc *textContent) string {
	if tc == nil {
		return ""
	}
	var b strings.Builder
	for _, el := range tc.TextElements {
		if el.TextRun != nil {
			b.WriteString(el.TextRun.Content)
		}
	}
	return b.String()
}

// resolveLayout maps a slide's layoutObjectId to the human layout name.
func (p *presentation) resolveLayout(s page) string {
	if s.SlideProperties == nil || s.SlideProperties.LayoutObjectID == "" {
		return ""
	}
	for _, l := range p.Layouts {
		if l.ObjectID == s.SlideProperties.LayoutObjectID {
			if l.LayoutProperties.Name != "" {
				return l.LayoutProperties.Name
			}
			return l.LayoutProperties.DisplayName
		}
	}
	return s.SlideProperties.LayoutObjectID
}

// findElementText returns the concatenated text of a page element by object id,
// searching every slide (recursing groups) and its notes page. Used by
// `text insert --append` to locate the append index.
func (p *presentation) findElementText(objectID string) (string, bool) {
	for _, s := range p.Slides {
		if txt, ok := findInElements(s.PageElements, objectID); ok {
			return txt, true
		}
		if s.SlideProperties != nil && s.SlideProperties.NotesPage != nil {
			if txt, ok := findInElements(s.SlideProperties.NotesPage.PageElements, objectID); ok {
				return txt, true
			}
		}
	}
	return "", false
}

func findInElements(elems []pageElement, objectID string) (string, bool) {
	for _, el := range elems {
		if el.ObjectID == objectID && el.Shape != nil {
			return shapeText(el.Shape.Text), true
		}
		if el.ElementGroup != nil {
			if txt, ok := findInElements(el.ElementGroup.Children, objectID); ok {
				return txt, true
			}
		}
	}
	return "", false
}

// utf16Len returns the length of s in UTF-16 code units — the unit Slides text
// indices count in.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// writeOutline renders the human-readable deck outline: per slide, its object
// id + layout, each text element's object id / placeholder type / text, and
// the speaker notes. This is the AI's working view of a deck; --json returns
// the raw Presentation instead.
func writeOutline(w io.Writer, p *presentation, filter slideFilter) {
	fmt.Fprintf(w, "Presentation: %s (%s)\n", p.Title, p.PresentationID)
	fmt.Fprintf(w, "URL: %s\n", presentationURL(p.PresentationID))
	fmt.Fprintf(w, "%d slide(s)\n", len(p.Slides))
	for i, s := range p.Slides {
		if !filter.matches(i, s.ObjectID) {
			continue
		}
		fmt.Fprintf(w, "\n[%d] slide=%s", i+1, s.ObjectID)
		if layoutName := p.resolveLayout(s); layoutName != "" {
			fmt.Fprintf(w, " layout=%s", layoutName)
		}
		fmt.Fprintln(w)
		writeElementsOutline(w, s.PageElements, "    ")
		writeNotesOutline(w, s)
	}
}

func writeElementsOutline(w io.Writer, elems []pageElement, indent string) {
	for _, el := range elems {
		switch {
		case el.Shape != nil:
			txt := strings.TrimRight(shapeText(el.Shape.Text), "\n")
			if txt == "" {
				continue
			}
			tag := el.ObjectID
			if el.Shape.Placeholder != nil && el.Shape.Placeholder.Type != "" {
				tag = fmt.Sprintf("%s [%s]", el.ObjectID, el.Shape.Placeholder.Type)
			}
			fmt.Fprintf(w, "%s%s: %s\n", indent, tag, oneLine(txt))
		case el.Table != nil:
			cells := tableText(el.Table)
			if cells == "" {
				continue
			}
			fmt.Fprintf(w, "%s%s [TABLE]: %s\n", indent, el.ObjectID, oneLine(cells))
		case el.ElementGroup != nil:
			fmt.Fprintf(w, "%s%s [GROUP]:\n", indent, el.ObjectID)
			writeElementsOutline(w, el.ElementGroup.Children, indent+"  ")
		}
	}
}

func writeNotesOutline(w io.Writer, s page) {
	if s.SlideProperties == nil || s.SlideProperties.NotesPage == nil {
		return
	}
	notesID := ""
	if s.SlideProperties.NotesPage.NotesProperties != nil {
		notesID = s.SlideProperties.NotesPage.NotesProperties.SpeakerNotesObjectID
	}
	if notesID == "" {
		return
	}
	if txt, ok := findInElements(s.SlideProperties.NotesPage.PageElements, notesID); ok {
		if trimmed := strings.TrimRight(txt, "\n"); trimmed != "" {
			fmt.Fprintf(w, "    notes: %s\n", oneLine(trimmed))
		}
	}
}

// tableText concatenates a table's cell text, newest-first row order.
func tableText(t *table) string {
	var parts []string
	for _, row := range t.TableRows {
		for _, cell := range row.TableCells {
			if txt := strings.TrimSpace(shapeText(cell.Text)); txt != "" {
				parts = append(parts, txt)
			}
		}
	}
	return strings.Join(parts, " | ")
}

// oneLine collapses internal newlines so each element stays on one outline row.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

// slideFilter selects a subset of slides for the outline: an empty filter
// matches all; index (1-based) or object id narrows to one slide.
type slideFilter struct {
	all      bool
	index    int    // 1-based; 0 = unset
	objectID string // exact slide object id; "" = unset
}

func (f slideFilter) matches(zeroBasedIndex int, objectID string) bool {
	if f.all {
		return true
	}
	if f.index > 0 {
		return zeroBasedIndex+1 == f.index
	}
	if f.objectID != "" {
		return objectID == f.objectID
	}
	return true
}
