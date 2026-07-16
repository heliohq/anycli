package microsoftoutlook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// graphEmailAddress is Graph's emailAddress complex type.
type graphEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// graphRecipient wraps an emailAddress (Graph recipient shape).
type graphRecipient struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

// graphBody is Graph's itemBody: contentType is "text" or "html".
type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// graphHeader is one internet message header.
type graphHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// graphAttachment mirrors Graph's fileAttachment (and the common inventory
// fields of other attachment kinds). contentBytes is standard base64 and is
// only populated for fileAttachment on a per-attachment fetch.
type graphAttachment struct {
	ODataType    string `json:"@odata.type"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	Size         int64  `json:"size"`
	ContentBytes string `json:"contentBytes"`
}

// graphMessage mirrors the Graph message resource fields this tool surfaces.
type graphMessage struct {
	ID                     string            `json:"id"`
	ConversationID         string            `json:"conversationId"`
	Subject                string            `json:"subject"`
	From                   *graphRecipient   `json:"from"`
	ToRecipients           []graphRecipient  `json:"toRecipients"`
	CcRecipients           []graphRecipient  `json:"ccRecipients"`
	BccRecipients          []graphRecipient  `json:"bccRecipients"`
	ReceivedDateTime       string            `json:"receivedDateTime"`
	SentDateTime           string            `json:"sentDateTime"`
	BodyPreview            string            `json:"bodyPreview"`
	Body                   graphBody         `json:"body"`
	IsRead                 bool              `json:"isRead"`
	IsDraft                bool              `json:"isDraft"`
	HasAttachments         bool              `json:"hasAttachments"`
	WebLink                string            `json:"webLink"`
	InternetMessageHeaders []graphHeader     `json:"internetMessageHeaders"`
	Attachments            []graphAttachment `json:"attachments"`
}

// headerFields is the $select list used when --headers is requested (Graph
// does not return internetMessageHeaders by default).
const headerFields = "id,conversationId,subject,from,toRecipients,ccRecipients,bccRecipients,receivedDateTime,sentDateTime,bodyPreview,body,isRead,isDraft,hasAttachments,webLink,internetMessageHeaders"

// fetchMessage retrieves one message with its attachment inventory. bodyKind
// ("text" | "html") drives the Prefer body-content-type; withHeaders adds the
// internetMessageHeaders $select.
func (s *Service) fetchMessage(ctx context.Context, token, id, bodyKind string, withHeaders bool) (*graphMessage, error) {
	q := url.Values{}
	q.Set("$expand", "attachments($select=id,name,contentType,size)")
	if withHeaders {
		q.Set("$select", headerFields)
	}
	headers := map[string]string{
		"Prefer": fmt.Sprintf("outlook.body-content-type=%q", bodyKind),
	}
	body, err := s.callH(ctx, token, "GET", "/me/messages/"+url.PathEscape(id), q, nil, headers)
	if err != nil {
		return nil, err
	}
	var m graphMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("microsoft-outlook: decode message: %w", err)
	}
	return &m, nil
}

// formatAddress renders "Name <addr>" or just the address.
func (a graphEmailAddress) String() string {
	addr := strings.TrimSpace(a.Address)
	name := strings.TrimSpace(a.Name)
	switch {
	case name != "" && addr != "" && !strings.EqualFold(name, addr):
		return fmt.Sprintf("%s <%s>", name, addr)
	case addr != "":
		return addr
	default:
		return name
	}
}

func formatRecipients(rs []graphRecipient) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		if v := r.EmailAddress.String(); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}

// renderMessage writes the human-readable form of a fetched message.
func (s *Service) renderMessage(w io.Writer, m *graphMessage) {
	fmt.Fprintf(w, "Id:       %s\n", m.ID)
	if m.From != nil {
		fmt.Fprintf(w, "From:     %s\n", m.From.EmailAddress.String())
	}
	if to := formatRecipients(m.ToRecipients); to != "" {
		fmt.Fprintf(w, "To:       %s\n", to)
	}
	if cc := formatRecipients(m.CcRecipients); cc != "" {
		fmt.Fprintf(w, "Cc:       %s\n", cc)
	}
	if m.Subject != "" {
		fmt.Fprintf(w, "Subject:  %s\n", m.Subject)
	}
	when := m.ReceivedDateTime
	if when == "" {
		when = m.SentDateTime
	}
	if when != "" {
		fmt.Fprintf(w, "Date:     %s\n", when)
	}
	fmt.Fprintf(w, "Read:     %t\n", m.IsRead)
	if len(m.InternetMessageHeaders) > 0 {
		fmt.Fprintln(w, "\nHeaders:")
		for _, h := range m.InternetMessageHeaders {
			fmt.Fprintf(w, "  %s: %s\n", h.Name, h.Value)
		}
	}
	fmt.Fprintln(w)
	if body := strings.TrimSpace(m.Body.Content); body != "" {
		fmt.Fprintln(w, body)
	} else if m.BodyPreview != "" {
		fmt.Fprintln(w, m.BodyPreview)
	} else {
		fmt.Fprintln(w, "(no body)")
	}
	if len(m.Attachments) > 0 {
		fmt.Fprintf(w, "\nAttachments (%d):\n", len(m.Attachments))
		for i, a := range m.Attachments {
			fmt.Fprintf(w, "  %d. %s  (%d bytes, %s)\n", i+1, a.Name, a.Size, a.ContentType)
		}
	}
}
