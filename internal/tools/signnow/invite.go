package signnow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newInviteSendCmd(token string) *cobra.Command {
	var to, email, subject, message, from string
	var noEmail bool
	cmd := &cobra.Command{
		Use:         "send <document-id>",
		Short:       "Send a document for signature (role-based field invite or free-form invite)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			docID := args[0]
			hasTo := strings.TrimSpace(to) != ""
			hasEmail := strings.TrimSpace(email) != ""
			if hasTo == hasEmail {
				return &usageError{msg: "invite send requires exactly one of --to (role-based field invite) or --email (free-form invite)"}
			}

			sender, err := s.resolveSender(cmd.Context(), token, from)
			if err != nil {
				return err
			}
			payload := map[string]any{"from": sender}
			if hasTo {
				var recipients []any
				if err := json.Unmarshal([]byte(to), &recipients); err != nil {
					return &usageError{msg: fmt.Sprintf("invite send: --to is not a JSON array: %v", err)}
				}
				payload["to"] = recipients
			} else {
				payload["to"] = email
			}
			if strings.TrimSpace(subject) != "" {
				payload["subject"] = subject
			}
			if strings.TrimSpace(message) != "" {
				payload["message"] = message
			}

			q := url.Values{}
			if noEmail {
				// Suppress SignNow's outbound signer email (embedded-style flow).
				q.Set("email", "disable")
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/document/"+url.PathEscape(docID)+"/invite", q, payload)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "JSON array of role recipients ({email, role, role_id, order}) for a field invite")
	cmd.Flags().StringVar(&email, "email", "", "single recipient email for a free-form invite (documents without fields)")
	cmd.Flags().StringVar(&from, "from", "", "sender email (default: the authenticated account's primary email)")
	cmd.Flags().StringVar(&subject, "subject", "", "invite email subject")
	cmd.Flags().StringVar(&message, "message", "", "invite email message")
	cmd.Flags().BoolVar(&noEmail, "no-email", false, "suppress the outbound signer email")
	return cmd
}

// resolveSender returns the explicit --from address when set, otherwise the
// authenticated account's primary email (SignNow requires a "from" on every
// invite). The lookup runs at most once per invocation.
func (s *Service) resolveSender(ctx context.Context, token, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	u, err := s.fetchUser(ctx, token)
	if err != nil {
		return "", err
	}
	if u.primaryEmail() == "" {
		return "", &apiError{msg: "invite send: could not resolve a sender email from the account; pass --from"}
	}
	return u.primaryEmail(), nil
}

func (s *Service) newInviteResendCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "resend <field-invite-id>",
		Short:       "Resend / remind a pending field invite",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if _, err := s.call(cmd.Context(), token, http.MethodPut, "/fieldinvite/"+url.PathEscape(id)+"/resend", nil, map[string]any{}); err != nil {
				return err
			}
			return s.emitJSON(map[string]any{"id": id, "status": "resent"})
		},
	}
	return cmd
}

func (s *Service) newInviteCancelCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "cancel <document-id>",
		Short:       "Cancel a document's field invite (recall a sent document)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			docID := args[0]
			if _, err := s.call(cmd.Context(), token, http.MethodPut, "/document/"+url.PathEscape(docID)+"/fieldinvitecancel", nil, map[string]any{}); err != nil {
				return err
			}
			return s.emitJSON(map[string]any{"id": docID, "status": "cancelled"})
		},
	}
	return cmd
}
