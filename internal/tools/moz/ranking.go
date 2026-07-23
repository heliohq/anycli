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

// newRankingKeywordsListCmd lists the keywords a site ranks in the top 50 for
// (data.site.ranking-keyword.list).
func (s *Service) newRankingKeywordsListCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "list",
		short:  "Keywords a site ranks top-50 for",
		method: "data.site.ranking-keyword.list",
	})
}

// newRankingKeywordsCountCmd returns the count of keywords a site ranks top-50
// for (data.site.ranking-keyword.count).
func (s *Service) newRankingKeywordsCountCmd(token string) *cobra.Command {
	var site, scope string
	cmd := &cobra.Command{
		Use:         "count",
		Short:       "Count of keywords a site ranks top-50 for",
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
