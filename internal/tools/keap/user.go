package keap

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := newGroupCmd("user", "Users (list, me)")
	cmd.AddCommand(
		s.newUserListCmd(token),
		s.newUserMeCmd(token),
	)
	return cmd
}

func (s *Service) newUserListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users (GET /v2/users)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/users", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newUserMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Get the authorizing user/tenant (GET /v2/oauth/connect/userinfo)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/oauth/connect/userinfo", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
