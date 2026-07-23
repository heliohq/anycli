package docusign

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Provider-neutral output shapes. Field names are snake_case and provider
// agnostic (not DocuSign's raw camelCase) so DocuSign output reads consistently
// with the other tools an assistant drives. --json emits these; the default
// human output is one summary line per row.

// envelopeView is the neutral projection of one DocuSign envelope.
type envelopeView struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Subject     string `json:"subject,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	SentAt      string `json:"sent_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Recipients  int    `json:"recipients,omitempty"`
}

// recipientView is the neutral projection of one envelope recipient.
type recipientView struct {
	Name         string `json:"name,omitempty"`
	Email        string `json:"email,omitempty"`
	Status       string `json:"status,omitempty"`
	Type         string `json:"type,omitempty"`
	RoutingOrder string `json:"routing_order,omitempty"`
	RecipientID  string `json:"recipient_id,omitempty"`
	SignedAt     string `json:"signed_at,omitempty"`
}

// templateView is the neutral projection of one reusable template.
type templateView struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Shared      string `json:"shared,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// emitJSON writes any value as a compact JSON line to stdout.
func emitJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("docusign: encode output: %v", err), err: err}
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// emitLine writes one plain-text summary line to stdout.
func emitLine(w io.Writer, parts ...string) {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	fmt.Fprintln(w, strings.Join(nonEmpty, "\t"))
}

// encodeQuery turns a map[string]string of query params into an encoded query
// string, skipping empty values. A nil or non-map argument yields "".
func encodeQuery(query any) string {
	params, ok := query.(map[string]string)
	if !ok || len(params) == 0 {
		return ""
	}
	values := url.Values{}
	for k, v := range params {
		if strings.TrimSpace(v) != "" {
			values.Set(k, v)
		}
	}
	return values.Encode()
}
