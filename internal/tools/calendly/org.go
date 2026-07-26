package calendly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newOrgMembersCmd wraps GET /organization_memberships to resolve teammates'
// user URIs (needed for availability/busy queries on colleagues). It scopes to
// the authenticated user's current organization.
func (s *Service) newOrgMembersCmd(token string) *cobra.Command {
	var email string
	var count int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "members",
		Short: "List organization memberships to resolve teammates' user URIs (GET /organization_memberships)",
		Long: "Turns a colleague's name or email into the user URI that\n" +
			"`availability busy --user`, `availability schedule list --user` and\n" +
			"`event list --user` need — there is no other lookup from a person to a URI\n" +
			"here. Scoped to the connected user's own `current_organization`, and\n" +
			"`--email` narrows it to one member. An account with no organization fails\n" +
			"outright rather than returning an empty list, which is a solo Calendly\n" +
			"account, not a transient error.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, orgURI, err := s.resolveMe(cmd.Context(), token)
			if err != nil {
				return err
			}
			if orgURI == "" {
				return &usageError{msg: "calendly: no current_organization on /users/me"}
			}
			q := url.Values{}
			q.Set("organization", orgURI)
			if email != "" {
				q.Set("email", email)
			}
			addPaging(q, count, pageToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/organization_memberships", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter memberships by member email")
	cmd.Flags().IntVar(&count, "count", 0, "page size (cursor pagination)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination cursor")
	return cmd
}
