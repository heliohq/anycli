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
		Use:         "list",
		Short:       "List team members (GET /users/search)",
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
		Use:         "profile",
		Short:       "Get the current token owner's profile (GET /users/api_profile)",
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
		Use:         "list",
		Short:       "List connected email accounts (GET /email_accounts)",
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
