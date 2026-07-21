package typeform

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd is `me` (GET /me): the authenticated account's identity (alias,
// email, language, and — on live payloads — user_id). Needs the accounts:read
// scope. Output JSON.
func (s *Service) newMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Retrieve the authenticated Typeform account (GET /me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}
