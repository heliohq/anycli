package moz

import (
	"github.com/spf13/cobra"
)

func (s *Service) newRankingKeywordsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("ranking-keywords", "Keywords a site ranks top-50 for")
	cmd.AddCommand(
		s.newRankingKeywordsListCmd(token),
		s.newRankingKeywordsCountCmd(token),
	)
	return cmd
}

// longRankingKeywordsList is this leaf's Long. It sits here rather than beside
// newTargetListCmd because the top-50 cutoff it describes is a property of this
// method, not of the shared target_query list shape.
const longRankingKeywordsList = "Covers only keywords where the target already ranks in the TOP 50, so a\n" +
	"term the site does not rank for is simply absent — absence here is not\n" +
	"evidence that a keyword is unranked-for-everyone, and this cannot answer\n" +
	"\"does my page rank for X\". Each keyword bills a row and --limit defaults\n" +
	"to 25; `ranking-keywords count` returns the total for about one row."

// newRankingKeywordsListCmd lists the keywords a site ranks in the top 50 for
// (data.site.ranking-keyword.list).
func (s *Service) newRankingKeywordsListCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "list",
		short:  "Keywords a site ranks top-50 for",
		long:   longRankingKeywordsList,
		method: "data.site.ranking-keyword.list",
	})
}

// newRankingKeywordsCountCmd returns the count of keywords a site ranks top-50
// for (data.site.ranking-keyword.count).
func (s *Service) newRankingKeywordsCountCmd(token string) *cobra.Command {
	var site, scope string
	cmd := &cobra.Command{
		Use:   "count",
		Short: "Count of keywords a site ranks top-50 for",
		Long: "Returns the total for roughly one row of quota, where paging\n" +
			"`ranking-keywords list` to the same answer would bill one row per\n" +
			"keyword. Size a pull with this before committing quota to the list. The\n" +
			"count obeys --scope, so a root_domain figure and a page figure for the\n" +
			"same site are both correct and very different.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "moz: --site is required"}
			}
			if err := checkScope(scope); err != nil {
				return err
			}
			data := map[string]any{"target_query": siteQuery(site, scope)}
			result, err := s.call(cmd.Context(), token, "data.site.ranking-keyword.count", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "site to count ranking keywords for")
	cmd.Flags().StringVar(&scope, "scope", "", "query scope: page|subdomain|root_domain (default: API default)")
	return cmd
}
