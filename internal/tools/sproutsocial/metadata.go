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
	for _, r := range []struct{ use, resource, short, long string }{
		{"tags", "tags", "List message/post tags (GET /v1/{cid}/metadata/customer/tags)", longMetadataTags},
		{"groups", "groups", "List profile groups (GET /v1/{cid}/metadata/customer/groups)", longMetadataGroups},
		{"users", "users", "List Sprout users (GET /v1/{cid}/metadata/customer/users)", longMetadataUsers},
		{"topics", "topics", "List listening topics (GET /v1/{cid}/metadata/customer/topics)", longMetadataTopics},
		{"teams", "teams", "List teams (GET /v1/{cid}/metadata/customer/teams)", longMetadataTeams},
		{"queues", "queues", "List publishing queues (GET /v1/{cid}/metadata/customer/queues)", longMetadataQueues},
	} {
		cmd.AddCommand(s.newMetadataResourceCmd(token, r.use, r.resource, r.short, r.long))
	}
	return cmd
}

// newMetadataClientCmd lists the customer ids the token can see. This is the
// only endpoint with no customer-id path segment.
func (s *Service) newMetadataClientCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "client",
		Short: "List the customers this token can access (GET /v1/metadata/client)",
		Long: "The only call in this tool that carries no customer id, which makes it the\n" +
			"one that still answers when the injected default is wrong or the token has\n" +
			"been repointed. Returns every customer id the token can reach; pass one of\n" +
			"them as the global `--customer-id` to aim the rest of the tool at it.",
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
		Use:   "profiles",
		Short: "List the customer's social profiles (GET /v1/{cid}/metadata/customer)",
		Long: "Returns the connected social profiles with the `customer_profile_id` values\n" +
			"that `--filter customer_profile_id.eq(...)` and\n" +
			"`publishing create --profile-id` both take, plus each profile's network and\n" +
			"handle. Start here before any profile-scoped analytics query — the ids are\n" +
			"Sprout's own and bear no relation to the network's account ids.",
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

// The six customer sub-resource Longs live next to the shared builder because
// it is the builder that fixes the endpoint shape they all describe: an
// unfiltered, unpaged GET whose only job is turning ids into names.
const (
	longMetadataTags = "Tags are the labels applied to inbox messages and published posts. This\n" +
		"returns the tag vocabulary with the ids that `--filter` clauses reference;\n" +
		"nothing in this tool applies or removes a tag."

	longMetadataGroups = "Groups bundle social profiles for publishing and reporting. The `group_id`\n" +
		"returned here is what `publishing create --group-id` requires, and it is a\n" +
		"different id space from `customer_profile_id`."

	longMetadataUsers = "The Sprout users on this customer, with the ids that show up as authors and\n" +
		"assignees elsewhere in the API. Use it to turn a bare numeric owner id on a\n" +
		"message or case into a person."

	longMetadataTopics = "Listening topics are the saved queries behind Sprout's social-listening\n" +
		"reports. This returns their ids and names only — no verb here runs a topic\n" +
		"or reads its results."

	longMetadataTeams = "Teams group Sprout users for routing and reporting. Returns team ids and\n" +
		"names, which is what resolves the team references carried on cases and inbox\n" +
		"messages."

	longMetadataQueues = "Publishing queues are the scheduling slots the Sprout app fills. Their ids\n" +
		"are returned for reference only: publishing through this API is draft-only,\n" +
		"so nothing here can place a post into a queue."
)

// newMetadataResourceCmd builds one GET /v1/{cid}/metadata/customer/<resource>
// command.
func (s *Service) newMetadataResourceCmd(token, use, resource, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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
