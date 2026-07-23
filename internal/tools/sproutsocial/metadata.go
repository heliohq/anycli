package sproutsocial

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMetadataCmd is the discovery/id-resolution group. `client` is the only
// call that carries no customer id (it returns the customer ids the token can
// see); every other metadata read is scoped to /v1/{cid}/metadata/customer[/…].
func (s *Service) newMetadataCmd(token string) *cobra.Command {
	cmd := newGroupCmd("metadata", "Account metadata (customers, profiles, tags, groups, users, topics, teams, queues)")
	cmd.AddCommand(
		s.newMetadataClientCmd(token),
		s.newMetadataProfilesCmd(token),
	)
	// Customer sub-resources all live under /metadata/customer/<r>.
	for _, r := range []struct{ use, resource, short string }{
		{"tags", "tags", "List message/post tags (GET /v1/{cid}/metadata/customer/tags)"},
		{"groups", "groups", "List profile groups (GET /v1/{cid}/metadata/customer/groups)"},
		{"users", "users", "List Sprout users (GET /v1/{cid}/metadata/customer/users)"},
		{"topics", "topics", "List listening topics (GET /v1/{cid}/metadata/customer/topics)"},
		{"teams", "teams", "List teams (GET /v1/{cid}/metadata/customer/teams)"},
		{"queues", "queues", "List publishing queues (GET /v1/{cid}/metadata/customer/queues)"},
	} {
		cmd.AddCommand(s.newMetadataResourceCmd(token, r.use, r.resource, r.short))
	}
	return cmd
}

// newMetadataClientCmd lists the customer ids the token can see. This is the
// only endpoint with no customer-id path segment.
func (s *Service) newMetadataClientCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "client",
		Short:       "List the customers this token can access (GET /v1/metadata/client)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/metadata/client", nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newMetadataProfilesCmd lists the customer's social profiles.
func (s *Service) newMetadataProfilesCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "profiles",
		Short:       "List the customer's social profiles (GET /v1/{cid}/metadata/customer)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/"+cid+"/metadata/customer", nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newMetadataResourceCmd builds one GET /v1/{cid}/metadata/customer/<resource>
// command.
func (s *Service) newMetadataResourceCmd(token, use, resource, short string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/"+cid+"/metadata/customer/"+resource, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
