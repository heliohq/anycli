package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountCmd reports account balance, rate limits, and pricing via the free
// appendix/user_data endpoint. It doubles as the free smoke / identity command:
// it is the only wrapped call that DataForSEO does not charge for.
func (s *Service) newAccountCmd(credential string) *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Account balance, rate limits, and pricing (free)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.do(cmd.Context(), credential, http.MethodGet, "/appendix/user_data", nil)
		},
	}
}
