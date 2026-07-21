package customerio

import (
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newSendEmailCmd(key string) *cobra.Command {
	var transactionalID, to, messageData, from, subject, body, plaintextBody, bcc, replyTo string
	var identifiers []string
	var disableRetention, queueDraft bool
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Send a transactional email (POST /v1/send/email)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ids, err := parseIdentifiers(identifiers)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"transactional_message_id": transactionalID,
				"to":                       to,
				"identifiers":              ids,
			}
			if messageData != "" {
				v, dErr := decodeJSONFlag("message-data", messageData)
				if dErr != nil {
					return dErr
				}
				payload["message_data"] = v
			}
			setIfPresent(payload, "from", from)
			setIfPresent(payload, "subject", subject)
			setIfPresent(payload, "body", body)
			setIfPresent(payload, "body_plain", plaintextBody)
			setIfPresent(payload, "bcc", bcc)
			setIfPresent(payload, "reply_to", replyTo)
			if disableRetention {
				payload["disable_message_retention"] = true
			}
			if queueDraft {
				payload["queue_draft"] = true
			}
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/send/email", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&transactionalID, "transactional-id", "", "transactional message id or template")
	cmd.Flags().StringVar(&to, "to", "", "recipient email address")
	cmd.Flags().StringArrayVar(&identifiers, "identifier", nil, "person identifier as key=value (id=, email=, cio_id=); repeatable")
	cmd.Flags().StringVar(&messageData, "message-data", "", "liquid template data as raw JSON object")
	cmd.Flags().StringVar(&from, "from", "", "override the From address")
	cmd.Flags().StringVar(&subject, "subject", "", "override the subject")
	cmd.Flags().StringVar(&body, "body", "", "override the HTML body")
	cmd.Flags().StringVar(&plaintextBody, "plaintext-body", "", "override the plaintext body")
	cmd.Flags().StringVar(&bcc, "bcc", "", "BCC address")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "Reply-To address")
	cmd.Flags().BoolVar(&disableRetention, "disable-message-retention", false, "do not retain the rendered message body")
	cmd.Flags().BoolVar(&queueDraft, "queue-draft", false, "queue the message as a draft instead of sending")
	_ = cmd.MarkFlagRequired("transactional-id")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("identifier")
	return cmd
}

// parseIdentifiers turns repeated key=value flags into the identifiers object
// Customer.io's send endpoint expects (e.g. {"email":"a@b.co"}). At least one
// pair is required and each must be key=value.
func parseIdentifiers(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, &usageError{msg: "--identifier must be key=value (e.g. email=jane@example.com)"}
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, &usageError{msg: "at least one --identifier key=value is required"}
	}
	return out, nil
}

// setIfPresent adds key→value to m only when value is non-empty.
func setIfPresent(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
