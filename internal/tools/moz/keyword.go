package moz

import (
	"github.com/spf13/cobra"
)

func (s *Service) newKeywordCmd(token string) *cobra.Command {
	cmd := newGroupCmd("keyword", "Keyword volume/difficulty, suggestions, and search intent")
	cmd.AddCommand(
		s.newKeywordMetricsCmd(token),
		s.newKeywordSuggestionsCmd(token),
		s.newKeywordIntentCmd(token),
	)
	return cmd
}

// serpQuery builds a Moz serp_query object from a keyword and optional locale.
// An empty locale is omitted so the API applies its documented default.
func serpQuery(keyword, locale string) map[string]any {
	q := map[string]any{"keyword": keyword}
	if locale != "" {
		q["locale"] = locale
	}
	return q
}

// newKeywordMetricsCmd fetches volume, difficulty, organic CTR, and priority
// for one keyword (data.keyword.metrics.fetch).
func (s *Service) newKeywordMetricsCmd(token string) *cobra.Command {
	var keyword, locale string
	cmd := &cobra.Command{
		Use:         "metrics",
		Short:       "Search volume, difficulty, organic CTR, and priority for a keyword",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keyword == "" {
				return &usageError{msg: "moz: --keyword is required"}
			}
			data := map[string]any{"serp_query": serpQuery(keyword, locale)}
			result, err := s.call(cmd.Context(), token, "data.keyword.metrics.fetch", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "keyword to fetch metrics for")
	cmd.Flags().StringVar(&locale, "locale", "", "locale, e.g. en-US (default: API default)")
	return cmd
}

// newKeywordSuggestionsCmd lists related keyword suggestions
// (data.keyword.suggestions.list).
func (s *Service) newKeywordSuggestionsCmd(token string) *cobra.Command {
	var keyword, locale string
	var limit int
	cmd := &cobra.Command{
		Use:         "suggestions",
		Short:       "Related keyword suggestions for a seed keyword",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keyword == "" {
				return &usageError{msg: "moz: --keyword is required"}
			}
			data := map[string]any{"serp_query": serpQuery(keyword, locale), "limit": limit}
			result, err := s.call(cmd.Context(), token, "data.keyword.suggestions.list", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "seed keyword")
	cmd.Flags().StringVar(&locale, "locale", "", "locale, e.g. en-US (default: API default)")
	cmd.Flags().IntVar(&limit, "limit", 25, "max suggestions to return (each returned row bills quota)")
	return cmd
}

// newKeywordIntentCmd classifies the search intent of a keyword
// (data.keyword.search.intent.fetch).
func (s *Service) newKeywordIntentCmd(token string) *cobra.Command {
	var keyword, locale string
	cmd := &cobra.Command{
		Use:         "intent",
		Short:       "Search-intent classification for a keyword",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keyword == "" {
				return &usageError{msg: "moz: --keyword is required"}
			}
			data := map[string]any{"serp_query": serpQuery(keyword, locale)}
			result, err := s.call(cmd.Context(), token, "data.keyword.search.intent.fetch", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "keyword to classify")
	cmd.Flags().StringVar(&locale, "locale", "", "locale, e.g. en-US (default: API default)")
	return cmd
}
