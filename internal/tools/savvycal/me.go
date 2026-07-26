package savvycal

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated user (GET /v1/me)",
		Long: "Returns the connected account's identity — id, name, email and default time\n" +
			"zone. It is the cheapest confirmation of WHICH SavvyCal account is about to\n" +
			"be booked or cancelled on, and the source of the account's own time zone,\n" +
			"which is not the same as the `--time-zone` an `event create` records for\n" +
			"the person booking.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
