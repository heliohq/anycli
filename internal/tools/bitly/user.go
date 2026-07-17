package bitly

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Authenticated user"}
	cmd.AddCommand(s.newUserGetCmd(token))
	return cmd
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the authenticated user (GET /user)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/user", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
