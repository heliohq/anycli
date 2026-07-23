package hootsuite

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd resolves the authenticated member (identity + org discovery).
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated member (GET /v1/me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newOrgListCmd lists the organizations the member belongs to.
func (s *Service) newOrgListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the member's organizations (GET /v1/me/organizations)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/organizations", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
