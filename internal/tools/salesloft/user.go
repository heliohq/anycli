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
		Long: "Users are the reps on the Salesloft team, never prospects — those are\n" +
			"`person` records. Their ids are what `--user-id` and `--owner-id` take\n" +
			"on the write commands, so this is the lookup before assigning a task,\n" +
			"reassigning ownership, or enrolling someone on another rep's behalf.\n" +
			"The standard list controls apply, and `--filter` reaches Salesloft's\n" +
			"documented user filters when the list is long.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "`--id` is required and is a USER id — a person id here reaches an\n" +
			"unrelated record or 404s, since the two id spaces are independent.\n" +
			"Returns the rep's name, email, team, role and whether the seat is still\n" +
			"active, which is worth checking before assigning them work.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
