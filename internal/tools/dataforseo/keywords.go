package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newKeywordsCmd is the `keywords` resource group: search volume, ideas,
// suggestions, difficulty, and search intent.
func (s *Service) newKeywordsCmd(credential string) *cobra.Command {
	kw := newGroupCmd("keywords", "Keyword research")
	kw.AddCommand(
		s.newKeywordsVolumeCmd(credential),
		s.newKeywordsIdeasCmd(credential),
		s.newKeywordsSuggestionsCmd(credential),
		s.newKeywordsDifficultyCmd(credential),
		s.newKeywordsIntentCmd(credential),
	)
	return kw
}

// newKeywordsVolumeCmd returns Google Ads search volume for a keyword list.
func (s *Service) newKeywordsVolumeCmd(credential string) *cobra.Command {
	var (
		keywords string
		tp       taskParams
	)
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Google Ads search volume for keywords",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/keywords_data/google_ads/search_volume/live", task)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated keywords (required)")
	_ = cmd.MarkFlagRequired("keywords")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newKeywordsIdeasCmd expands seed keywords into related keyword ideas.
func (s *Service) newKeywordsIdeasCmd(credential string) *cobra.Command {
	var (
		keywords string
		tp       taskParams
		limit    int
	)
	cmd := &cobra.Command{
		Use:   "ideas",
		Short: "Related keyword ideas for seed keywords",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/keyword_ideas/live", task)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated seed keywords (required)")
	_ = cmd.MarkFlagRequired("keywords")
	cmd.Flags().IntVar(&limit, "limit", 0, "max keyword ideas (default 700, max 1000)")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newKeywordsSuggestionsCmd returns long-tail suggestions for one seed keyword.
func (s *Service) newKeywordsSuggestionsCmd(credential string) *cobra.Command {
	var (
		keyword string
		tp      taskParams
		limit   int
	)
	cmd := &cobra.Command{
		Use:   "suggestions",
		Short: "Long-tail keyword suggestions for a seed keyword",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keyword": keyword}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/keyword_suggestions/live", task)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "seed keyword (required)")
	_ = cmd.MarkFlagRequired("keyword")
	cmd.Flags().IntVar(&limit, "limit", 0, "max suggestions (default 100, max 1000)")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newKeywordsDifficultyCmd returns bulk keyword difficulty scores.
func (s *Service) newKeywordsDifficultyCmd(credential string) *cobra.Command {
	var (
		keywords string
		tp       taskParams
	)
	cmd := &cobra.Command{
		Use:   "difficulty",
		Short: "Bulk keyword difficulty scores",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/bulk_keyword_difficulty/live", task)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated keywords (required)")
	_ = cmd.MarkFlagRequired("keywords")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newKeywordsIntentCmd classifies keywords by search intent (language only —
// the endpoint takes no location).
func (s *Service) newKeywordsIntentCmd(credential string) *cobra.Command {
	var (
		keywords string
		tp       taskParams
	)
	cmd := &cobra.Command{
		Use:   "intent",
		Short: "Search intent classification for keywords",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/search_intent/live", task)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated keywords (required)")
	_ = cmd.MarkFlagRequired("keywords")
	registerLanguageOnly(cmd, &tp)
	return cmd
}
