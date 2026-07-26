package pipedrive

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserGroup builds the v1 users family: "me" (identity check) and "list"
// (owner assignment lookup). Users have no v2 equivalent.
func (s *Service) newUserGroup(c *caller) *cobra.Command {
	g := newGroupCmd("user", "Look up users (v1)")
	g.AddCommand(
		&cobra.Command{
			Use:   "me",
			Short: "Get the authenticated user",
			Long: "The user the connected token acts as, and the company that token is scoped\n" +
				"to. Its id is what --owner-id means when assigning a deal, person,\n" +
				"organization or activity to \"me\" — every ownership flag in the tool takes\n" +
				"a user id, never a name.",
			Args:        cobra.NoArgs,
			Annotations: readOnly,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.run(cmd.Context(), http.MethodGet, "/api/v1/users/me", nil, nil)
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List all users in the company",
			Long: "Returns EVERY user in the company in one call: a v1 endpoint with no\n" +
				"pagination, which is why there is no --cursor, --start or --limit here.\n" +
				"These ids are what every --owner-id flag takes. Deactivated users are\n" +
				"included, so check a record's active flag before assigning work to one.",
			Args:        cobra.NoArgs,
			Annotations: readOnly,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.run(cmd.Context(), http.MethodGet, "/api/v1/users", nil, nil)
			},
		},
	)
	return g
}

// newSearchCmd builds the top-level cross-entity search over the v2 itemSearch
// endpoint ("find the Acme deal"). --types maps to item_types.
func (s *Service) newSearchCmd(c *caller) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search across deals, persons, organizations, and leads",
		Long: "The cross-entity search: --types takes a comma-separated list of deal,\n" +
			"person, organization, lead, product, file and project, and omitting it\n" +
			"searches all of them — one call instead of running each entity's own\n" +
			"search verb. --term is required and needs at least 2 characters unless\n" +
			"--exact-match is also passed. --exact-match is a bare boolean: write\n" +
			"--exact-match, never --exact-match true. --limit caps at 100 and paging is\n" +
			"by --cursor.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			term, _ := cmd.Flags().GetString("term")
			q.Set("term", term)
			if v, _ := cmd.Flags().GetString("types"); v != "" {
				q.Set("item_types", v)
			}
			for _, f := range []string{"fields", "limit", "cursor"} {
				if cmd.Flags().Changed(f) {
					v, _ := cmd.Flags().GetString(f)
					q.Set(f, v)
				}
			}
			if cmd.Flags().Changed("exact-match") {
				exact, _ := cmd.Flags().GetBool("exact-match")
				if exact {
					q.Set("exact_match", "true")
				}
			}
			return c.run(cmd.Context(), http.MethodGet, "/api/v2/itemSearch", q, nil)
		},
	}
	cmd.Flags().String("term", "", "search term (required, min 2 chars)")
	cmd.Flags().String("types", "", "comma-separated item types (deal,person,organization,lead,product,file,project)")
	cmd.Flags().String("fields", "", "comma-separated fields to search within")
	cmd.Flags().String("limit", "", "max results (max 100)")
	cmd.Flags().String("cursor", "", "pagination cursor")
	cmd.Flags().Bool("exact-match", false, "require an exact term match")
	_ = cmd.MarkFlagRequired("term")
	return cmd
}
