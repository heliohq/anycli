package loops

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEmailCmd groups transactional-email operations (send a templated email,
// list templates).
func (s *Service) newEmailCmd(key string) *cobra.Command {
	cmd := newGroup("email", "Transactional email (send, list templates)")
	cmd.AddCommand(
		s.newEmailSendCmd(key),
		s.newEmailListCmd(key),
	)
	return cmd
}

func (s *Service) newEmailSendCmd(key string) *cobra.Command {
	var email, transactionalID, dataVarsJSON, attachmentsJSON, idempotencyKey string
	var dataVariable []string
	var addToAudience bool
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a templated transactional email (POST /v1/transactional)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"email": email, "transactionalId": transactionalID}
			dataVars := map[string]any{}
			if dataVarsJSON != "" {
				raw, err := decodeJSONObject("data-variables-json", dataVarsJSON)
				if err != nil {
					return err
				}
				for k, v := range raw {
					dataVars[k] = v
				}
			}
			kv, err := parseKeyValues("data-variable", dataVariable)
			if err != nil {
				return err
			}
			for k, v := range kv {
				dataVars[k] = v
			}
			if len(dataVars) > 0 {
				body["dataVariables"] = dataVars
			}
			if cmd.Flags().Changed("add-to-audience") {
				body["addToAudience"] = addToAudience
			}
			if attachmentsJSON != "" {
				arr, err := decodeJSONArray("attachments-json", attachmentsJSON)
				if err != nil {
					return err
				}
				body["attachments"] = arr
			}
			resp, err := s.callIdempotent(cmd.Context(), key, http.MethodPost, "/v1/transactional", nil, body, idempotencyKey)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "recipient email address (required)")
	cmd.Flags().StringVar(&transactionalID, "transactional-id", "", "transactional email template id (required)")
	cmd.Flags().StringArrayVar(&dataVariable, "data-variable", nil, "template data variable key=value, typed-coerced (repeatable)")
	cmd.Flags().StringVar(&dataVarsJSON, "data-variables-json", "", "template data variables as a raw JSON object")
	cmd.Flags().BoolVar(&addToAudience, "add-to-audience", false, "create the contact in the audience if absent")
	cmd.Flags().StringVar(&attachmentsJSON, "attachments-json", "", "attachments as a raw JSON array (must be enabled by Loops support)")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency-Key header (409 on replay)")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("transactional-id")
	return cmd
}

func (s *Service) newEmailListCmd(key string) *cobra.Command {
	var perPage, cursor string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List transactional email templates (GET /v1/transactional; deprecated by Loops)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage != "" {
				q.Set("perPage", perPage)
			}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/transactional", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&perPage, "per-page", "", "results per page (10-50, default 20)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}
