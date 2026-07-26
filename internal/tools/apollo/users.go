package apollo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newUsersCmd builds the `users` group: resolve team member ids (needed to set
// deal owners) and the current token owner's profile.
func (s *Service) newUsersCmd(token string) *cobra.Command {
	cmd := newGroupCmd("users", "Look up team members")
	cmd.AddCommand(
		s.newUsersListCmd(token),
		s.newUsersProfileCmd(token),
	)
	return cmd
}

// newUsersListCmd wraps GET /users/search.
func (s *Service) newUsersListCmd(token string) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List team members (GET /users/search)",
		Long: "Returns the users on the connected Apollo team with their user ids; that\n" +
			"id is what `deals create --owner-id` expects. Page with --page (1-based)\n" +
			"and --per-page — there are no name or email filters, so fetch a page and\n" +
			"match locally.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyPageQuery(q, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/search", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page, &perPage)
	return cmd
}

// newUsersProfileCmd wraps GET /users/api_profile — the token owner's identity,
// the same endpoint Helio's OAuth identity resolver reads.
func (s *Service) newUsersProfileCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Get the current token owner's profile (GET /users/api_profile)",
		Long: "Identifies which Apollo user and team the connected credential belongs to.\n" +
			"It takes no arguments and spends no enrichment credits, which makes it the\n" +
			"cheap connectivity probe: a 401 here means the credential itself is\n" +
			"rejected, whereas a 403 on another command is that endpoint's\n" +
			"master-API-key gate.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/api_profile", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newEmailAccountsCmd builds the `email-accounts` group: list the sending
// mailboxes an agent references when enrolling contacts into a sequence.
func (s *Service) newEmailAccountsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("email-accounts", "List connected sending mailboxes")
	cmd.AddCommand(s.newEmailAccountsListCmd(token))
	return cmd
}

// newEmailAccountsListCmd wraps GET /email_accounts.
func (s *Service) newEmailAccountsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List connected email accounts (GET /email_accounts)",
		Long: "Returns the mailboxes connected to the Apollo account together with the\n" +
			"ids that `sequences add --email-account-id` expects. Run it before\n" +
			"enrolling anyone: a sequence enrollment has no mailbox to send from until\n" +
			"one of these ids is supplied.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/email_accounts", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
