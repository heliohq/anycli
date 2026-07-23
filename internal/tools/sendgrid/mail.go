package sendgrid

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newMailCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "mail", Short: "Transactional mail (send)"}
	cmd.AddCommand(s.newMailSendCmd(token, region))
	return cmd
}

// newMailSendCmd wraps POST /v3/mail/send. The common case is driven by
// ergonomic flags (--to/--from/--subject/--text/--html or --template-id +
// --data), and --json is an escape hatch carrying a full v3 Mail Send body.
//
// Quirk (DESIGN §2): a successful send returns 202 Accepted with an EMPTY body
// and the tracking id in the X-Message-Id response header. This handler treats
// 202 as success (never decodes the empty body) and emits a synthetic
// acceptance object. 202 is API-layer acceptance, not delivery.
func (s *Service) newMailSendCmd(token string, region *string) *cobra.Command {
	var to, cc, bcc []string
	var from, fromName, subject, text, html, templateID, dataJSON, replyTo, fullJSON string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send an email (POST /v3/mail/send)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := buildMailPayload(mailInput{
				to: to, cc: cc, bcc: bcc,
				from: from, fromName: fromName, subject: subject,
				text: text, html: html, templateID: templateID,
				dataJSON: dataJSON, replyTo: replyTo, fullJSON: fullJSON,
			})
			if err != nil {
				return err
			}
			resp, err := s.do(cmd.Context(), token, *region, http.MethodPost, "/mail/send", nil, payload)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]any{
				"status":     "accepted",
				"message_id": resp.Header.Get("X-Message-Id"),
			})
		},
	}
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient email (repeatable)")
	cmd.Flags().StringArrayVar(&cc, "cc", nil, "cc email (repeatable)")
	cmd.Flags().StringArrayVar(&bcc, "bcc", nil, "bcc email (repeatable)")
	cmd.Flags().StringVar(&from, "from", "", "verified sender email (see `sender list`)")
	cmd.Flags().StringVar(&fromName, "from-name", "", "sender display name")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject (omit for a template that sets it)")
	cmd.Flags().StringVar(&text, "text", "", "text/plain body")
	cmd.Flags().StringVar(&html, "html", "", "text/html body")
	cmd.Flags().StringVar(&templateID, "template-id", "", "dynamic template id (see `template list`)")
	cmd.Flags().StringVar(&dataJSON, "data", "", "dynamic_template_data JSON object")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "reply-to email")
	cmd.Flags().StringVar(&fullJSON, "json-body", "", "full v3 Mail Send body JSON (escape hatch; overrides other flags)")
	return cmd
}

// mailInput holds the flag values buildMailPayload assembles into a v3 body.
type mailInput struct {
	to, cc, bcc                                                        []string
	from, fromName, subject, text, html, templateID, dataJSON, replyTo string
	fullJSON                                                           string
}

// buildMailPayload assembles a v3 Mail Send body from flags, or decodes the
// full-body escape hatch. The escape hatch wins outright when provided.
func buildMailPayload(in mailInput) (any, error) {
	if in.fullJSON != "" {
		return decodeJSONFlag("json-body", in.fullJSON)
	}
	if len(in.to) == 0 {
		return nil, fmt.Errorf("sendgrid: mail send requires --to (or --json-body)")
	}
	if in.from == "" {
		return nil, fmt.Errorf("sendgrid: mail send requires --from (a verified sender)")
	}
	if in.templateID == "" && in.subject == "" {
		return nil, fmt.Errorf("sendgrid: mail send requires --subject (or --template-id)")
	}
	if in.templateID == "" && in.text == "" && in.html == "" {
		return nil, fmt.Errorf("sendgrid: mail send requires --text or --html (or --template-id)")
	}

	personalization := map[string]any{"to": emailList(in.to)}
	if len(in.cc) > 0 {
		personalization["cc"] = emailList(in.cc)
	}
	if len(in.bcc) > 0 {
		personalization["bcc"] = emailList(in.bcc)
	}
	if in.dataJSON != "" {
		data, err := decodeJSONFlag("data", in.dataJSON)
		if err != nil {
			return nil, err
		}
		personalization["dynamic_template_data"] = data
	}

	from := map[string]any{"email": in.from}
	if in.fromName != "" {
		from["name"] = in.fromName
	}
	body := map[string]any{
		"personalizations": []any{personalization},
		"from":             from,
	}
	if in.subject != "" {
		body["subject"] = in.subject
	}
	if content := mailContent(in.text, in.html); len(content) > 0 {
		body["content"] = content
	}
	if in.templateID != "" {
		body["template_id"] = in.templateID
	}
	if in.replyTo != "" {
		body["reply_to"] = map[string]any{"email": in.replyTo}
	}
	return body, nil
}

// emailList maps addresses to the v3 [{email}] array shape.
func emailList(addresses []string) []any {
	out := make([]any, 0, len(addresses))
	for _, addr := range addresses {
		out = append(out, map[string]any{"email": addr})
	}
	return out
}

// mailContent builds the ordered content[] array; SendGrid requires text/plain
// before text/html when both are present.
func mailContent(text, html string) []any {
	var content []any
	if text != "" {
		content = append(content, map[string]any{"type": "text/plain", "value": text})
	}
	if html != "" {
		content = append(content, map[string]any{"type": "text/html", "value": html})
	}
	return content
}
