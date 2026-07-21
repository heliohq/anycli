package pandadoc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// docSummary carries the fields the concise list/item renderers surface.
type docSummary struct {
	ID     string `json:"id"`
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Email  string `json:"email"`
}

// id returns the resource id, tolerating providers that key it as uuid.
func (d docSummary) id() string {
	if d.ID != "" {
		return d.ID
	}
	return d.UUID
}

// listEnvelope is the common PandaDoc list wrapper ({"results":[...]}). Contacts
// may instead return a bare array; renderList handles both.
type listEnvelope struct {
	Results []docSummary `json:"results"`
}

// renderList prints one concise line per item from a list response. It accepts
// either the {"results":[…]} envelope or a bare JSON array.
func (s *Service) renderList(body []byte) error {
	var env listEnvelope
	var items []docSummary
	if err := json.Unmarshal(body, &env); err == nil && env.Results != nil {
		items = env.Results
	} else {
		var bare []docSummary
		if err := json.Unmarshal(body, &bare); err == nil {
			items = bare
		}
	}
	if len(items) == 0 {
		fmt.Fprintln(s.stdout(), "(no results)")
		return nil
	}
	for _, it := range items {
		fmt.Fprintln(s.stdout(), summaryLine(it))
	}
	return nil
}

// renderItem prints one concise line for a single-object response.
func (s *Service) renderItem(body []byte) error {
	var it docSummary
	if err := json.Unmarshal(body, &it); err != nil {
		// Fall back to raw passthrough when the shape is unexpected.
		return s.emitJSON(body)
	}
	fmt.Fprintln(s.stdout(), summaryLine(it))
	return nil
}

// summaryLine renders a single resource as tab-separated id / status / label.
func summaryLine(it docSummary) string {
	parts := []string{it.id()}
	if it.Status != "" {
		parts = append(parts, it.Status)
	}
	label := it.Name
	if label == "" {
		label = it.Email
	}
	if label != "" {
		parts = append(parts, label)
	}
	return strings.Join(parts, "\t")
}
