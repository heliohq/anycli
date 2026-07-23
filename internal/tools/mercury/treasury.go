package mercury

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTreasuryGetCmd lists the organization's treasury (money-market / T-bill)
// accounts (GET /treasury). The endpoint returns an accounts array plus a page
// cursor; it is surfaced as a get verb because there is one treasury view per
// organization.
func (s *Service) newTreasuryGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get treasury accounts and balances (GET /treasury)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/treasury", nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "accounts", "page")
		},
	}
	return cmd
}
