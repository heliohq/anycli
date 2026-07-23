package calendly

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Current user: URI, organization URI, scheduling_url, timezone (GET /users/me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
