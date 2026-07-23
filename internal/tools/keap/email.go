package keap

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newEmailCmd(token string) *cobra.Command {
	cmd := newGroupCmd("email", "Emails (send, list)")
	cmd.AddCommand(
		s.newEmailSendCmd(token),
		s.newEmailListCmd(token),
	)
	return cmd
}

func (s *Service) newEmailSendCmd(token string) *cobra.Command {
	var contacts []string
	var subject, userID, html, plain, jsonBody string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a one-off email to contacts (POST /v2/emails:send)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if len(contacts) > 0 {
				body["contacts"] = contacts
			}
			if subject != "" {
				body["subject"] = subject
			}
			if userID != "" {
				body["user_id"] = userID
			}
			if html != "" {
				body["html_content"] = html
			}
			if plain != "" {
				body["plain_content"] = plain
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			for _, required := range []string{"contacts", "subject", "user_id"} {
				if _, ok := body[required]; !ok {
					return &usageError{msg: "email send requires --contact (at least one), --subject, and --user-id"}
				}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/emails:send", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&contacts, "contact", nil, "recipient contact id (repeatable, required)")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject (required)")
	cmd.Flags().StringVar(&userID, "user-id", "", "sending user id (required)")
	cmd.Flags().StringVar(&html, "html", "", "HTML body content")
	cmd.Flags().StringVar(&plain, "plain", "", "plain-text body content")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return cmd
}

func (s *Service) newEmailListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List sent/recorded emails (GET /v2/emails)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/emails", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}
