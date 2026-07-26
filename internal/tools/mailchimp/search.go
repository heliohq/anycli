package mailchimp

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSearchCmd builds the top-level search group: members and campaigns.
func (s *Service) newSearchCmd(r *requester) *cobra.Command {
	group := newGroupCmd("search", "Search across audiences and campaigns")
	group.AddCommand(
		s.newSearchMembersCmd(r),
		s.newSearchCampaignsCmd(r),
	)
	return group
}

func (s *Service) newSearchMembersCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "Search members across audiences (GET /search-members)",
		Long: "`--query` is required. This is the only read that spans ALL audiences at\n" +
			"once, so it answers \"is this address anywhere in the account\" without paging\n" +
			"each audience. The response splits into `exact_matches` and `full_search`; a\n" +
			"complete email address lands in `exact_matches`. Every hit carries its own\n" +
			"`list_id`, which is what the member commands then need.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runSearch(cmd, r, "/search-members")
		},
	}
	registerSearchFlags(cmd)
	return cmd
}

func (s *Service) newSearchCampaignsCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaigns",
		Short: "Search campaigns (GET /search-campaigns)",
		Long: "`--query` is required and matches campaign titles and subject lines across\n" +
			"every status, drafts included. Use it to recover a campaign id when only the\n" +
			"subject wording is known; when the filter is a status rather than a phrase,\n" +
			"`campaign list --status sent` is the cheaper path.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runSearch(cmd, r, "/search-campaigns")
		},
	}
	registerSearchFlags(cmd)
	return cmd
}

// runSearch is the shared search executor: it requires --query and forwards the
// optional --fields projection.
func (s *Service) runSearch(cmd *cobra.Command, r *requester, path string) error {
	query, _ := cmd.Flags().GetString("query")
	if query == "" {
		return &usageError{msg: "search requires --query"}
	}
	q := url.Values{}
	q.Set("query", query)
	if f, _ := cmd.Flags().GetString("fields"); f != "" {
		q.Set("fields", f)
	}
	body, err := r.do(cmd.Context(), http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// registerSearchFlags wires the shared --query / --fields flags onto a search
// command.
func registerSearchFlags(cmd *cobra.Command) {
	cmd.Flags().String("query", "", "search query (required)")
	cmd.Flags().String("fields", "", "comma-separated fields projection (passthrough)")
}
