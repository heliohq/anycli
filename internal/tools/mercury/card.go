package mercury

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCardListCmd lists the debit/credit cards on an account
// (GET /account/{accountId}/cards).
func (s *Service) newCardListCmd(token string) *cobra.Command {
	var account string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the cards on an account (GET /account/{id}/cards)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			if account == "" {
				return &usageError{msg: "--account is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/account/"+url.PathEscape(account)+"/cards", nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "cards")
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "account id to list cards for (required)")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}
