package resend

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newEmailCmd(key string) *cobra.Command {
	cmd := newGroupCmd("email", "Send and manage emails (send, batch, get, update, cancel)")
	cmd.AddCommand(
		s.newEmailSendCmd(key),
		s.newEmailBatchCmd(key),
		s.newEmailGetCmd(key),
		s.newEmailUpdateCmd(key),
		s.newEmailCancelCmd(key),
	)
	return cmd
}

func (s *Service) newEmailSendCmd(key string) *cobra.Command {
	var from, subject, html, text, cc, bcc, replyTo, scheduledAt string
	var to []string
	var attachmentsJSON, tagsJSON, headersJSON, idempotencyKey string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a single email (POST /emails)",
		Long: "--from, --to and --subject are required here; a body is not, though\n" +
			"Resend needs --html or --text, and sending both gives clients a\n" +
			"plain-text fallback. --to is repeatable up to 50 recipients and they all\n" +
			"see each other — separate messages are separate calls.\n" +
			"\n" +
			"--idempotency-key sets the Idempotency-Key header, which dedupes a\n" +
			"repeated send for 24 hours; it is the only protection against double\n" +
			"delivery on a retry, and there is none by default. --attachments,\n" +
			"--tags and --headers take raw JSON passed straight through. Success here\n" +
			"means Resend accepted the message, not that it was delivered —\n" +
			"`email get` reports what happened afterwards.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"from":    from,
				"subject": subject,
			}
			// Resend accepts to as string|array (max 50). Send a scalar for a
			// single recipient, an array for many.
			if len(to) == 1 {
				body["to"] = to[0]
			} else {
				body["to"] = to
			}
			if html != "" {
				body["html"] = html
			}
			if text != "" {
				body["text"] = text
			}
			if cc != "" {
				body["cc"] = cc
			}
			if bcc != "" {
				body["bcc"] = bcc
			}
			if replyTo != "" {
				body["reply_to"] = replyTo
			}
			if scheduledAt != "" {
				body["scheduled_at"] = scheduledAt
			}
			if attachmentsJSON != "" {
				v, err := decodeJSONFlag("attachments", attachmentsJSON)
				if err != nil {
					return err
				}
				body["attachments"] = v
			}
			if tagsJSON != "" {
				v, err := decodeJSONFlag("tags", tagsJSON)
				if err != nil {
					return err
				}
				body["tags"] = v
			}
			if headersJSON != "" {
				v, err := decodeJSONFlag("headers", headersJSON)
				if err != nil {
					return err
				}
				body["headers"] = v
			}
			var headers map[string]string
			if idempotencyKey != "" {
				headers = map[string]string{"Idempotency-Key": idempotencyKey}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/emails", body, headers)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "sender, `Name <addr>` form; addr must be on a verified domain")
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient (repeatable, max 50)")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject")
	cmd.Flags().StringVar(&html, "html", "", "HTML body")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body")
	cmd.Flags().StringVar(&cc, "cc", "", "cc recipient(s)")
	cmd.Flags().StringVar(&bcc, "bcc", "", "bcc recipient(s)")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "reply-to address")
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "schedule time: ISO-8601 or natural language (e.g. \"in 1 min\")")
	cmd.Flags().StringVar(&attachmentsJSON, "attachments", "", "attachments JSON array (raw passthrough)")
	cmd.Flags().StringVar(&tagsJSON, "tags", "", "tags JSON array (raw passthrough)")
	cmd.Flags().StringVar(&headersJSON, "headers", "", "custom headers JSON object (raw passthrough)")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency-Key request header (unique per request, 24h TTL)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("subject")
	return cmd
}

func (s *Service) newEmailBatchCmd(key string) *cobra.Command {
	var emailsJSON string
	cmd := &cobra.Command{
		Use:         "batch",
		Short:       "Send up to 100 emails in one call (POST /emails/batch)",
		Annotations: writeAction,
		Long: "--emails is a JSON ARRAY of complete email objects, and their fields are\n" +
			"the API's own snake_case names (scheduled_at, reply_to), not this tool's\n" +
			"flag names. Attachments are REJECTED by this endpoint, so any email\n" +
			"carrying one has to go through `email send` individually; per-email\n" +
			"scheduled_at and tags are fine. There is no idempotency key on this\n" +
			"path, so a retried batch delivers twice.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("emails", emailsJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/emails/batch", payload, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&emailsJSON, "emails", "", "JSON array of email objects (max 100; no attachments)")
	_ = cmd.MarkFlagRequired("emails")
	return cmd
}

func (s *Service) newEmailGetCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve a sent email's delivery status (GET /emails/{id})",
		Long: "Takes the id `email send` returned. This is the delivery record —\n" +
			"status, recipients, timestamps — and the only way to learn whether a\n" +
			"message actually landed, bounced or drew a complaint. The status\n" +
			"advances asynchronously, so a read immediately after sending reports an\n" +
			"early state rather than a final one.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/emails/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newEmailUpdateCmd(key string) *cobra.Command {
	var scheduledAt string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Reschedule a not-yet-sent email (PATCH /emails/{id})",
		Long: "Rescheduling is the only edit available: --scheduled-at is required and\n" +
			"no recipient, subject or body can be changed after the send was created.\n" +
			"It applies only while the email is still waiting — one already sent\n" +
			"cannot be moved or recalled.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if scheduledAt != "" {
				body["scheduled_at"] = scheduledAt
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPatch, "/emails/"+args[0], body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "new schedule time: ISO-8601 or natural language")
	_ = cmd.MarkFlagRequired("scheduled-at")
	return cmd
}

func (s *Service) newEmailCancelCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a scheduled email (POST /emails/{id}/cancel)",
		Long: "Works only on an email still waiting for its scheduled time. Once the\n" +
			"message has gone out nothing in this tool retracts it, so a cancel\n" +
			"attempted after the scheduled moment fails rather than unsending\n" +
			"anything.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/emails/"+args[0]+"/cancel", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
