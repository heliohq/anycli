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
		Long: "--keywords takes a comma-separated list and the whole list is answered in\n" +
			"ONE charged call, which makes batching far cheaper than a call per\n" +
			"keyword. The figures are Google Ads monthly averages: bucketed, lagging,\n" +
			"and scoped to --location and --language rather than global.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/keywords_data/google_ads/search_volume/live", task)
		},
	}
	cmd.Annotations = readOnly
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
		Long: "Expands seeds BROADLY: semantically related terms that need not contain the\n" +
			"seed word at all. `keywords suggestions` is the narrow counterpart, and\n" +
			"the two answer different questions. --keywords accepts several seeds in\n" +
			"one call. --limit defaults to 700 and tops out at 1000.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/keyword_ideas/live", task)
		},
	}
	cmd.Annotations = readOnly
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
		Long: "Narrow expansion: the returned terms CONTAIN the seed, which is what makes\n" +
			"this the long-tail command where `keywords ideas` is the lateral one. The\n" +
			"flag is --keyword, singular — unlike every other command in this group,\n" +
			"only one seed is accepted. --limit defaults to 100 and tops out at 1000.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keyword": keyword}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/keyword_suggestions/live", task)
		},
	}
	cmd.Annotations = readOnly
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
		Long: "Scores how contested each keyword is, 0-100, for the whole comma-separated\n" +
			"list in one charged call. The score is relative to --location and\n" +
			"--language: the same keyword is not equally hard in every market, so a\n" +
			"score read against the wrong location is worse than no score.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/bulk_keyword_difficulty/live", task)
		},
	}
	cmd.Annotations = readOnly
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
		Long: "Classifies each keyword as informational, navigational, commercial or\n" +
			"transactional — the signal that decides whether a page should explain or\n" +
			"sell. This endpoint is LANGUAGE-ONLY: it has no --location, unlike every\n" +
			"other command in this group.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keywords": splitKeywords(keywords)}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/search_intent/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated keywords (required)")
	_ = cmd.MarkFlagRequired("keywords")
	registerLanguageOnly(cmd, &tp)
	return cmd
}
