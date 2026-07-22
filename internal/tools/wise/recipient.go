package wise

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newRecipientListCmd looks up saved recipient / beneficiary accounts.
// GET /v2/accounts?profile={id}&currency={c} — v2 returns the richer
// accountSummary / displayFields / hash the AI can render or diff.
func (s *Service) newRecipientListCmd(token string) *cobra.Command {
	var profile, currency string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List saved recipient accounts (GET /v2/accounts)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if profile != "" {
				q.Set("profile", profile)
			}
			if currency != "" {
				q.Set("currency", currency)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/accounts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile id filter")
	cmd.Flags().StringVar(&currency, "currency", "", "target currency filter")
	return cmd
}
