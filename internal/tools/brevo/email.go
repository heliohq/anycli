package brevo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newEmailSendCmd builds `brevo email send` — POST /smtp/email, a one-off
// transactional/event email (distinct from a scheduled bulk campaign).
func (s *Service) newEmailSendCmd(apiKey string) *cobra.Command {
	var (
		to, cc, bcc                      []string
		toJSON                           string
		senderEmail, senderName, replyTo string
		senderID, templateID             int
		subject, html, text, paramsJSON  string
		tags                             []string
	)
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a transactional email (POST /smtp/email)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}

			// Recipients: --to-json (full control) overrides plain --to.
			if toJSON != "" {
				arr, err := decodeJSONArrayFlag("to-json", toJSON)
				if err != nil {
					return err
				}
				body["to"] = arr
			} else if len(to) > 0 {
				body["to"] = emailEntries(to)
			}

			// Sender: --sender-id wins over --sender-email/--sender-name.
			if cmd.Flags().Changed("sender-id") {
				body["sender"] = map[string]any{"id": senderID}
			} else if senderEmail != "" {
				sender := map[string]any{"email": senderEmail}
				if senderName != "" {
					sender["name"] = senderName
				}
				body["sender"] = sender
			}

			if subject != "" {
				body["subject"] = subject
			}
			if html != "" {
				body["htmlContent"] = html
			}
			if text != "" {
				body["textContent"] = text
			}
			if cmd.Flags().Changed("template-id") {
				body["templateId"] = templateID
			}
			if paramsJSON != "" {
				params, err := decodeJSONObjectFlag("params-json", paramsJSON)
				if err != nil {
					return err
				}
				body["params"] = params
			}
			if len(cc) > 0 {
				body["cc"] = emailEntries(cc)
			}
			if len(bcc) > 0 {
				body["bcc"] = emailEntries(bcc)
			}
			if replyTo != "" {
				body["replyTo"] = map[string]any{"email": replyTo}
			}
			if len(tags) > 0 {
				body["tags"] = tags
			}

			resp, err := s.call(cmd.Context(), apiKey, http.MethodPost, "/smtp/email", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient email (repeatable)")
	cmd.Flags().StringVar(&toJSON, "to-json", "", "recipients as a raw JSON array (overrides --to), e.g. [{\"email\":\"x\",\"name\":\"Y\"}]")
	cmd.Flags().StringVar(&senderEmail, "sender-email", "", "sender email (must be a verified sender)")
	cmd.Flags().StringVar(&senderName, "sender-name", "", "sender display name")
	cmd.Flags().IntVar(&senderID, "sender-id", 0, "verified sender id (alternative to --sender-email)")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject (required unless --template-id supplies one)")
	cmd.Flags().StringVar(&html, "html", "", "HTML body (htmlContent)")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body (textContent)")
	cmd.Flags().IntVar(&templateID, "template-id", 0, "transactional template id")
	cmd.Flags().StringVar(&paramsJSON, "params-json", "", "template params as a raw JSON object")
	cmd.Flags().StringArrayVar(&cc, "cc", nil, "cc recipient email (repeatable)")
	cmd.Flags().StringArrayVar(&bcc, "bcc", nil, "bcc recipient email (repeatable)")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "reply-to email")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "message tag (repeatable)")
	return cmd
}
