package missive

import (
	"encoding/json"
	"fmt"
)

// Missive paginates two different ways (see DESIGN §2). Contacts and contact
// books use limit+offset; conversations and their sub-resources use a limit
// plus an `until` timestamp cursor, and a page may return *more* than limit —
// so the agent must page on the returned cursor, never on the item count.
//
// Both list shapes are normalized to a provider-neutral envelope on stdout:
//
//	{"items": [...], "next_offset": <int|null>}   // offset-paged lists
//	{"items": [...], "next_until":  <cursor|null>} // cursor-paged lists
//
// The cursor value is passed through verbatim (Missive returns a Unix
// timestamp), so the agent feeds it straight back via --until.

// emitOffsetList extracts the items under resourceKey and computes next_offset:
// offset+len(items) when the page came back full (len == limit, so more may
// exist), otherwise null.
func (s *Service) emitOffsetList(body []byte, resourceKey string, offset, limit int) error {
	items, err := extractItems(body, resourceKey)
	if err != nil {
		return err
	}
	var next any
	if limit > 0 && len(items) >= limit {
		next = offset + len(items)
	}
	return s.emitListEnvelope(items, "next_offset", next)
}

// emitUntilList extracts the items under resourceKey and sets next_until to the
// cursorField value of the last (oldest) item, or null on an empty page. A
// Missive page may exceed limit, so the presence of items — not their count —
// decides whether there is another page to fetch.
func (s *Service) emitUntilList(body []byte, resourceKey, cursorField string) error {
	items, err := extractItems(body, resourceKey)
	if err != nil {
		return err
	}
	var next any
	if n := len(items); n > 0 {
		if cursor, ok := items[n-1][cursorField]; ok {
			next = cursor
		}
	}
	return s.emitListEnvelope(items, "next_until", next)
}

// extractItems pulls the resource array out of Missive's single-key list
// wrapper ({"conversations":[...]}). An absent key is treated as an empty page.
func extractItems(body []byte, resourceKey string) ([]map[string]json.RawMessage, error) {
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("missive: decode %s list: %v", resourceKey, err), err: err}
	}
	raw, ok := wrap[resourceKey]
	if !ok {
		return nil, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("missive: decode %s items: %v", resourceKey, err), err: err}
	}
	return items, nil
}

// emitListEnvelope writes {"items":[...], "<cursorKey>":<next>} to stdout. Item
// objects are re-marshaled from their raw fields, so values pass through
// unchanged; a nil next renders as JSON null.
func (s *Service) emitListEnvelope(items []map[string]json.RawMessage, cursorKey string, next any) error {
	if items == nil {
		items = []map[string]json.RawMessage{}
	}
	b, err := json.Marshal(map[string]any{"items": items, cursorKey: next})
	if err != nil {
		return &apiError{msg: fmt.Sprintf("missive: encode list envelope: %v", err), err: err}
	}
	return s.emit(b)
}
