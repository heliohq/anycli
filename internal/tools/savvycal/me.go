package savvycal

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated user (GET /v1/me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
