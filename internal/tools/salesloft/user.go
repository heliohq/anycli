package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newUserCmd groups the read-only user lookups used to resolve teammates for
// user_id parameters.
func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := newGroupCmd("user", "Look up team users")
	cmd.AddCommand(
		s.newUserListCmd(token),
		s.newUserGetCmd(token),
	)
	return cmd
}

func (s *Service) newUserListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List team users (GET /v2/users)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch one user (GET /v2/users/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
