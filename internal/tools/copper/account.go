package copper

import (
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// newAccountCmd exposes the org-level account record (GET /account) — whoami at
// the Copper-account level.
func (s *Service) newAccountCmd(token string) *cobra.Command {
	group := newGroupCmd("account", "Copper account (organization)")
	group.AddCommand(&cobra.Command{
		Use:         "get",
		Short:       "Get the Copper account (GET /account)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/account", nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	})
	return group
}

// newUserCmd exposes the Copper users: the OAuth identity (GET /users/me), the
// full user list (GET /users), and a single user by id (GET /users/{id}).
func (s *Service) newUserCmd(token string) *cobra.Command {
	group := newGroupCmd("user", "Copper users")
	group.AddCommand(
		&cobra.Command{
			Use:         "me",
			Short:       "Get the authenticated Copper user (GET /users/me)",
			Args:        cobra.NoArgs,
			Annotations: readOnly,
			RunE: func(cmd *cobra.Command, _ []string) error {
				resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			},
		},
		&cobra.Command{
			Use:         "list",
			Short:       "List Copper users (GET /users)",
			Args:        cobra.NoArgs,
			Annotations: readOnly,
			RunE: func(cmd *cobra.Command, _ []string) error {
				resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users", nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			},
		},
		s.newUserGetCmd(token),
	)
	return group
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one Copper user by id (GET /users/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/"+strconv.Itoa(id), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper user id")
	return cmd
}
