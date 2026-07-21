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
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.run(cmd.Context(), http.MethodGet, "/api/v1/users/me", nil, nil)
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List all users in the company",
			Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
