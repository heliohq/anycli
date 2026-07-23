package rocketreach

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountCmd is `account` (GET /api/v2/account/): retrieve the authenticated
// account, including the credit_usage[] balance. Cheap and non-consuming — the
// agent calls it to check budget before spending credits on lookups. Also the
// connect-time verify endpoint on the Helio side.
func (s *Service) newAccountCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "account",
		Short:       "Get the authenticated account and credit balance (GET /account/)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/api/v2/account/", nil, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
}
