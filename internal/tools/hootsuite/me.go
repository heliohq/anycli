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
		Long: "The identity call, and the cheapest check that the credential is still live —\n" +
			"Hootsuite access tokens are short-lived, so a 401 here means the connection\n" +
			"needs refreshing rather than that the request was wrong. It returns the member\n" +
			"record alone: the organizations they belong to come from `org list`, and the\n" +
			"accounts they can actually post to from `profile list`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "An organization is Hootsuite's tenant boundary — teams, approval workflows and\n" +
			"social profiles all belong to one. Most members are in exactly one, and no\n" +
			"other command in this tool takes an organization id as an argument, so this is\n" +
			"a context read (\"which tenant am I acting inside\") rather than a step on the\n" +
			"way to posting.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/organizations", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
