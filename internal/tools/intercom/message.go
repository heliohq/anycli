package intercom

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMessageCmd builds the message resource group: admin-initiated outbound
// messages (proactive outreach) via POST /messages. An in-app or email message
// is sent from an admin to a contact.
func (s *Service) newMessageCmd(token string) *cobra.Command {
	cmd := newGroupCmd("message", "Admin-initiated outbound messages")
	cmd.AddCommand(s.newMessageSendCmd(token))
	return cmd
}

func (s *Service) newMessageSendCmd(token string) *cobra.Command {
	var messageType, subject, body, template, fromAdminID, toContactID, toEmail, bodyJSON string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send an admin-initiated message to a contact (POST /messages)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, fromAdminID)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"message_type": messageType,
				"body":         body,
				"from":         map[string]any{"type": "admin", "id": admin},
			}
			if messageType == "email" && subject != "" {
				payload["subject"] = subject
			}
			if template != "" {
				payload["template"] = template
			}
			to, err := messageTarget(toContactID, toEmail)
			if err != nil {
				return err
			}
			if to != nil {
				payload["to"] = to
			}
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/messages", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&messageType, "message-type", "inapp", "message type: inapp|email")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject (email message-type only)")
	cmd.Flags().StringVar(&body, "body", "", "message body")
	cmd.Flags().StringVar(&template, "template", "", "email template: plain|personal")
	cmd.Flags().StringVar(&fromAdminID, "from-admin-id", "", "sending admin id (defaults to the /me admin)")
	cmd.Flags().StringVar(&toContactID, "to-contact-id", "", "recipient contact id")
	cmd.Flags().StringVar(&toEmail, "to-email", "", "recipient email (alternative to --to-contact-id)")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw message JSON (merged; overrides the scalar flags)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// messageTarget builds the {type:"user", id|email} recipient object, preferring
// an explicit contact id over an email. Returns nil when neither is supplied
// (the caller may still provide the target through --body-json).
func messageTarget(contactID, email string) (map[string]any, error) {
	switch {
	case contactID != "":
		return map[string]any{"type": "user", "id": contactID}, nil
	case email != "":
		return map[string]any{"type": "user", "email": email}, nil
	default:
		return nil, nil
	}
}
