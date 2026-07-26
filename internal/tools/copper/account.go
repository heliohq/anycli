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
		Use:   "get",
		Short: "Get the Copper account (GET /account)",
		Long: "The organization the credential belongs to — the Copper tenant, not a person.\n" +
			"Worth one call to confirm which CRM is being read before writing anything,\n" +
			"since the id vocabularies that `lookup` returns are per-account and a payload\n" +
			"built against one account is meaningless in another. For the individual behind\n" +
			"the token, `user me` is the read.",
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
			Use:   "me",
			Short: "Get the authenticated Copper user (GET /users/me)",
			Long: "The identity anchor: the id returned here is what `--assignee-id` filters take\n" +
				"to answer \"what is assigned to me\", and what a create payload's `assignee_id`\n" +
				"should carry to file a record under the connected user. Nothing else in the\n" +
				"tool defaults to that user, so read it rather than guessing.",
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
			Use:   "list",
			Short: "List Copper users (GET /users)",
			Long: "The CRM's seat directory, and the lookup that turns a bare `assignee_id` on a\n" +
				"record into a name. A plain GET rather than a search POST, so it takes no\n" +
				"filters — read the page and match locally. These ids are also what a create or\n" +
				"update payload sets `assignee_id` to.",
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
		Use:   "get",
		Short: "Get one Copper user by id (GET /users/{id})",
		Long: "`--id` is required and is a numeric Copper user id, typically one taken from\n" +
			"another record's `assignee_id` or `owner_id`. For the connected user's own\n" +
			"record, `user me` needs no id; for more than a couple of lookups, one\n" +
			"`user list` is cheaper than repeated calls here.",
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
