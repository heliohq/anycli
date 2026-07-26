package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd fetches the authenticated user (GET /v2/me) — identity check and the
// bundle's identity probe.
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Fetch the authenticated user (GET /v2/me)",
		Long: "The record of the user this connection acts as: id, name, email, team\n" +
			"and role. That `id` is the implicit `user_id` on `cadence add-person`\n" +
			"and the value to pass as `--user-id` or `--owner-id` when work should\n" +
			"stay attributed to this user. It is also the cheapest confirmation that\n" +
			"the token is live and which Salesloft team it reaches.",
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
