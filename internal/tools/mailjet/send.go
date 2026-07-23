package mailjet

import (
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// newSendCmd sends a transactional email through the Send API v3.1
// (POST /v3.1/send). The v3.1 contract is a Messages[] array; this command
// sends exactly one message built from the flags. The response (per-message
// Status + recipient MessageID/MessageUUID) is emitted verbatim.
func (s *Service) newSendCmd(basic string) *cobra.Command {
	var fromEmail, fromName, subject, text, html, variablesJSON string
	var to, cc, bcc []string
	var templateID int64
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a transactional email (POST /v3.1/send)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			if len(to) == 0 {
				return &usageError{msg: "at least one --to recipient is required"}
			}
			if templateID == 0 && text == "" && html == "" {
				return &usageError{msg: "provide --text, --html, or --template-id"}
			}
			message := map[string]any{
				"From": recipient(fromEmail, fromName),
				"To":   recipients(to),
			}
			if subject != "" {
				message["Subject"] = subject
			}
			if text != "" {
				message["TextPart"] = text
			}
			if html != "" {
				message["HTMLPart"] = html
			}
			if len(cc) > 0 {
				message["Cc"] = recipients(cc)
			}
			if len(bcc) > 0 {
				message["Bcc"] = recipients(bcc)
			}
			if templateID != 0 {
				message["TemplateID"] = templateID
				message["TemplateLanguage"] = true
			}
			if variablesJSON != "" {
				v, err := decodeJSONFlag("variables-json", variablesJSON)
				if err != nil {
					return err
				}
				message["Variables"] = v
			}
			payload := map[string]any{"Messages": []any{message}}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodPost, "/v3.1/send", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fromEmail, "from-email", "", "sender email (must be a validated Mailjet sender)")
	cmd.Flags().StringVar(&fromName, "from-name", "", "sender display name (optional)")
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient, \"email\" or \"Name <email>\" (repeatable)")
	cmd.Flags().StringArrayVar(&cc, "cc", nil, "cc recipient, \"email\" or \"Name <email>\" (repeatable)")
	cmd.Flags().StringArrayVar(&bcc, "bcc", nil, "bcc recipient, \"email\" or \"Name <email>\" (repeatable)")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body")
	cmd.Flags().StringVar(&html, "html", "", "HTML body")
	cmd.Flags().Int64Var(&templateID, "template-id", 0, "send using a Mailjet template ID")
	cmd.Flags().StringVar(&variablesJSON, "variables-json", "", "template variables as a JSON object (raw passthrough)")
	_ = cmd.MarkFlagRequired("from-email")
	return cmd
}

// recipient builds a v3.1 {"Email","Name"} object; Name is omitted when empty.
func recipient(email, name string) map[string]any {
	r := map[string]any{"Email": email}
	if name != "" {
		r["Name"] = name
	}
	return r
}

// recipients parses each "email" or "Name <email>" entry into a v3.1 recipient.
func recipients(entries []string) []any {
	out := make([]any, 0, len(entries))
	for _, entry := range entries {
		email, name := parseAddress(entry)
		out = append(out, recipient(email, name))
	}
	return out
}

// parseAddress splits "Name <email>" into (email, name); a bare token is the
// email with no name.
func parseAddress(entry string) (email, name string) {
	entry = strings.TrimSpace(entry)
	open := strings.LastIndex(entry, "<")
	close := strings.LastIndex(entry, ">")
	if open >= 0 && close > open {
		email = strings.TrimSpace(entry[open+1 : close])
		name = strings.TrimSpace(entry[:open])
		return email, name
	}
	return entry, ""
}
