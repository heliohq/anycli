package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// apiMsg mirrors the Gmail API message resource (format=full).
type apiMsg struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId"`
	LabelIDs     []string `json:"labelIds"`
	Snippet      string   `json:"snippet"`
	SizeEstimate int64    `json:"sizeEstimate"`
	Payload      *apiPart `json:"payload"`
}

// apiPart is one node of the MIME part tree.
type apiPart struct {
	PartID   string      `json:"partId"`
	MimeType string      `json:"mimeType"`
	Filename string      `json:"filename"`
	Headers  []apiHeader `json:"headers"`
	Body     apiPartBody `json:"body"`
	Parts    []*apiPart  `json:"parts"`
}

type apiHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type apiPartBody struct {
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
	Data         string `json:"data"`
}

// attachmentInfo is one entry of a message's attachment inventory.
type attachmentInfo struct {
	AttachmentID string `json:"attachmentId"`
	PartID       string `json:"partId"`
	Filename     string `json:"filename"`
	MimeType     string `json:"mimeType,omitempty"`
	Size         int64  `json:"size"`
}

// messageView is the parsed projection emitted for `messages get`,
// `threads get`, and `drafts get` (--json contract).
type messageView struct {
	ID           string            `json:"id"`
	ThreadID     string            `json:"threadId"`
	LabelIDs     []string          `json:"labelIds,omitempty"`
	SizeEstimate int64             `json:"size_estimate"`
	Headers      map[string]string `json:"headers"`
	AllHeaders   []apiHeader       `json:"allHeaders,omitempty"`
	BodyType     string            `json:"bodyType,omitempty"`
	Body         string            `json:"body,omitempty"`
	Attachments  []attachmentInfo  `json:"attachments"`
}

// coreHeaders are the headers always surfaced on a parsed message.
var coreHeaders = []string{"From", "To", "Cc", "Bcc", "Reply-To", "Date", "Subject", "Message-ID", "In-Reply-To", "References"}

func parseMessage(body []byte) (*apiMsg, error) {
	var m apiMsg
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("gmail: decode message: %w", err)
	}
	return &m, nil
}

// fetchMessage retrieves one message in format=full.
func (s *Service) fetchMessage(ctx context.Context, token, id string) (*apiMsg, error) {
	q := url.Values{"format": {"full"}}
	body, err := s.call(ctx, token, "GET", "/users/me/messages/"+url.PathEscape(id), q, nil)
	if err != nil {
		return nil, err
	}
	return parseMessage(body)
}

// header returns the first header with the given name, case-insensitively.
func (m *apiMsg) header(name string) string {
	if m.Payload == nil {
		return ""
	}
	for _, h := range m.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// walkParts visits the payload part tree depth-first.
func walkParts(p *apiPart, visit func(*apiPart)) {
	if p == nil {
		return
	}
	visit(p)
	for _, child := range p.Parts {
		walkParts(child, visit)
	}
}

// bodyOfKind returns the decoded body of the first inline part with the given
// MIME type ("text/plain" or "text/html").
func (m *apiMsg) bodyOfKind(mimeType string) (string, error) {
	var found *apiPart
	walkParts(m.Payload, func(p *apiPart) {
		if found == nil && p.Filename == "" && strings.EqualFold(p.MimeType, mimeType) && p.Body.Data != "" {
			found = p
		}
	})
	if found == nil {
		return "", nil
	}
	decoded, err := decodeBase64URL(found.Body.Data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// resolveBody returns the message body of the requested kind ("text" or
// "html"), falling back to the other kind when the requested one is absent.
// The returned kind names what was actually found ("" = no body at all).
func (m *apiMsg) resolveBody(kind string) (body, actualKind string, err error) {
	order := []string{"text/plain", "text/html"}
	if kind == "html" {
		order = []string{"text/html", "text/plain"}
	}
	for _, mt := range order {
		body, err = m.bodyOfKind(mt)
		if err != nil {
			return "", "", err
		}
		if body != "" {
			if mt == "text/html" {
				return body, "html", nil
			}
			return body, "text", nil
		}
	}
	return "", "", nil
}

// attachments returns the attachment inventory of a message: every part that
// carries an attachmentId.
func (m *apiMsg) attachments() []attachmentInfo {
	var out []attachmentInfo
	walkParts(m.Payload, func(p *apiPart) {
		if p.Body.AttachmentID == "" {
			return
		}
		out = append(out, attachmentInfo{
			AttachmentID: p.Body.AttachmentID,
			PartID:       p.PartID,
			Filename:     p.Filename,
			MimeType:     p.MimeType,
			Size:         p.Body.Size,
		})
	})
	return out
}

// buildView projects an API message into the parsed view.
func buildView(m *apiMsg, bodyKind string, allHeaders bool) (messageView, error) {
	body, actual, err := m.resolveBody(bodyKind)
	if err != nil {
		return messageView{}, err
	}
	view := messageView{
		ID:           m.ID,
		ThreadID:     m.ThreadID,
		LabelIDs:     m.LabelIDs,
		SizeEstimate: m.SizeEstimate,
		Headers:      map[string]string{},
		BodyType:     actual,
		Body:         body,
		Attachments:  m.attachments(),
	}
	for _, name := range coreHeaders {
		if v := m.header(name); v != "" {
			view.Headers[name] = v
		}
	}
	if allHeaders && m.Payload != nil {
		view.AllHeaders = m.Payload.Headers
	}
	return view, nil
}

// renderMessage writes the human-readable form of a parsed message.
func renderMessage(w io.Writer, view messageView) {
	fmt.Fprintf(w, "Id:      %s\n", view.ID)
	fmt.Fprintf(w, "Thread:  %s\n", view.ThreadID)
	if len(view.LabelIDs) > 0 {
		fmt.Fprintf(w, "Labels:  %s\n", strings.Join(view.LabelIDs, ", "))
	}
	if view.SizeEstimate > 0 {
		fmt.Fprintf(w, "Size:    %d bytes\n", view.SizeEstimate)
	}
	for _, name := range []string{"From", "To", "Cc", "Bcc", "Reply-To", "Date", "Subject"} {
		if v := view.Headers[name]; v != "" {
			fmt.Fprintf(w, "%-8s %s\n", name+":", v)
		}
	}
	if len(view.AllHeaders) > 0 {
		fmt.Fprintln(w, "\nHeaders:")
		for _, h := range view.AllHeaders {
			fmt.Fprintf(w, "  %s: %s\n", h.Name, h.Value)
		}
	}
	fmt.Fprintln(w)
	if view.Body != "" {
		fmt.Fprintln(w, view.Body)
	} else {
		fmt.Fprintln(w, "(no body)")
	}
	if len(view.Attachments) > 0 {
		fmt.Fprintf(w, "\nAttachments (%d):\n", len(view.Attachments))
		for _, a := range view.Attachments {
			fmt.Fprintf(w, "  %s  %s  (%d bytes, %s)\n", a.AttachmentID, a.Filename, a.Size, a.MimeType)
		}
	}
}
