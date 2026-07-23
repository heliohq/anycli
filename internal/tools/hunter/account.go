package hunter

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountCmd wraps GET /account: plan, searches/verifications used vs
// available, and the monthly reset_date. Free of charge — the cheapest call to
// answer "how many searches do I have left?". This is also the endpoint Helio's
// bundle uses for API-key identity verification.
func (s *Service) newAccountCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "account",
		Short:       "Show account plan and quota usage (GET /account)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/account", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
