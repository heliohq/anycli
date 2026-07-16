package forms

import (
	"fmt"
	"strings"
)

// extractFormID accepts either a bare Forms API formId or a Forms edit URL
// (https://docs.google.com/forms/d/<formId>/edit) and returns the formId.
//
// The responder link form (/forms/d/e/<encodedId>/viewform) carries a
// public-facing encoded id that is NOT the API formId — forms.get on it 404s.
// That shape is rejected with an explicit hint to use the edit link instead.
func extractFormID(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("forms: empty form id")
	}

	// Responder / published link: /forms/d/e/<id>/viewform. Reject early —
	// the "e/" id is not an API formId.
	if strings.Contains(raw, "/forms/d/e/") || strings.Contains(raw, "/d/e/") {
		return "", fmt.Errorf("forms: %q is a responder link; the API needs the EDIT link (https://docs.google.com/forms/d/<id>/edit) or the bare form id", input)
	}

	const marker = "/forms/d/"
	if idx := strings.Index(raw, marker); idx >= 0 {
		rest := raw[idx+len(marker):]
		id := rest
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			id = rest[:slash]
		}
		if q := strings.IndexAny(id, "?#"); q >= 0 {
			id = id[:q]
		}
		if id == "" {
			return "", fmt.Errorf("forms: could not extract a form id from %q", input)
		}
		return id, nil
	}

	// Any other URL shape is not a recognizable Forms link.
	if strings.Contains(raw, "://") || strings.Contains(raw, "/") {
		return "", fmt.Errorf("forms: %q is not a Forms edit link; pass the bare form id or https://docs.google.com/forms/d/<id>/edit", input)
	}
	return raw, nil
}
