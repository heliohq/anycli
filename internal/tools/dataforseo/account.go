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
		Long: "The only charged-surface call DataForSEO bills nothing for, which makes it\n" +
			"both the balance check and a free credential smoke test.\n" +
			"`result[0].money.balance` is the remaining USD; the same response carries\n" +
			"the rate limits and the per-endpoint price list, so an unexpected `cost`\n" +
			"can be explained from here before the next call is made.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.do(cmd.Context(), credential, http.MethodGet, "/appendix/user_data", nil)
		},
	}
}
