package ahrefs

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

const (
	keywordOverviewDefaultSelect = "keyword,volume,difficulty,cpc,global_volume,traffic_potential,parent_topic,intents"
	keywordIdeasDefaultSelect    = "keyword,volume,difficulty,cpc,global_volume,intents"
)

// ideaKinds maps the `keyword ideas --kind` value to its keywords-explorer
// endpoint. matching = broad-match expansions, related = "also rank for",
// suggestions = autocomplete-style suggestions.
var ideaKinds = map[string]string{
	"matching":    "/keywords-explorer/matching-terms",
	"related":     "/keywords-explorer/related-terms",
	"suggestions": "/keywords-explorer/search-suggestions",
}

// newKeywordCmd builds the `keyword` group over Keywords Explorer: `overview`
// (metrics for explicit keywords), `ideas` (research fan-out), and
// `volume-history` (a single keyword's search-volume trend).
func (s *Service) newKeywordCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "keyword", Short: "Keyword research (Keywords Explorer)"}
	cmd.AddCommand(
		s.newKeywordOverviewCmd(token),
		s.newKeywordIdeasCmd(token),
		s.newKeywordVolumeHistoryCmd(token),
	)
	return cmd
}

// newKeywordOverviewCmd wraps GET /keywords-explorer/overview: volume, KD, CPC
// etc. for explicit --keywords in a --country. Requires select+country+keywords.
func (s *Service) newKeywordOverviewCmd(token string) *cobra.Command {
	var keywords, country string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Volume/KD/CPC for explicit keywords (GET /keywords-explorer/overview)",
		Long: "Takes keywords you already have — --keywords is a COMMA-SEPARATED list,\n" +
			"and both it and --country are required. It expands nothing: for finding\n" +
			"new terms use `keyword ideas`. Batch the list rather than calling once per\n" +
			"keyword, since every request bills at least the 50-unit minimum. The\n" +
			"default fields include `traffic_potential` and `parent_topic`, which say\n" +
			"more about whether a term is worth targeting than volume alone.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keywords == "" {
				return &usageError{msg: "ahrefs: --keywords is required"}
			}
			if country == "" {
				return &usageError{msg: "ahrefs: --country is required"}
			}
			q := url.Values{}
			q.Set("country", country)
			q.Set("keywords", keywords)
			rf.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/keywords-explorer/overview", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated keywords (required)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code (required)")
	registerRowFlags(cmd, &rf, keywordOverviewDefaultSelect, false)
	return cmd
}

// newKeywordIdeasCmd wraps the three keyword-research endpoints behind --kind.
// Each requires select+country+keywords (seed terms).
func (s *Service) newKeywordIdeasCmd(token string) *cobra.Command {
	var keywords, country, kind string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:   "ideas",
		Short: "Keyword ideas: matching|related|suggestions (Keywords Explorer)",
		Long: "--kind picks the expansion and defaults to `matching`: matching returns\n" +
			"broad-match terms containing the seed, related returns what people ranking\n" +
			"for the seed also rank for, and suggestions returns autocomplete-style\n" +
			"completions. They are different endpoints, not different sorts of one\n" +
			"result. --keywords (comma-separated seeds) and --country are both\n" +
			"required, and `--limit` still defaults to 10, so raise it deliberately\n" +
			"when a real idea list is needed.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, ok := ideaKinds[kind]
			if !ok {
				return &usageError{msg: "ahrefs: --kind must be one of matching|related|suggestions"}
			}
			if keywords == "" {
				return &usageError{msg: "ahrefs: --keywords is required"}
			}
			if country == "" {
				return &usageError{msg: "ahrefs: --country is required"}
			}
			q := url.Values{}
			q.Set("country", country)
			q.Set("keywords", keywords)
			rf.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&keywords, "keywords", "", "comma-separated seed keywords (required)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code (required)")
	cmd.Flags().StringVar(&kind, "kind", "matching", "idea kind: matching|related|suggestions")
	registerRowFlags(cmd, &rf, keywordIdeasDefaultSelect, false)
	return cmd
}

// newKeywordVolumeHistoryCmd wraps GET /keywords-explorer/volume-history: the
// search-volume trend for one keyword. Requires country+keyword; no select.
func (s *Service) newKeywordVolumeHistoryCmd(token string) *cobra.Command {
	var keyword, country, from, to string
	cmd := &cobra.Command{
		Use:   "volume-history",
		Short: "Search-volume trend for one keyword (GET /keywords-explorer/volume-history)",
		Long: "Takes ONE keyword in --keyword, not the comma-separated --keywords the\n" +
			"other keyword commands use, and --country is required. --from and --to\n" +
			"bound the history as YYYY-MM-DD and are optional; with neither, Ahrefs\n" +
			"returns its full retained range. This is the command that separates a\n" +
			"seasonal keyword from one in genuine decline — an overview volume is a\n" +
			"single point in time.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keyword == "" {
				return &usageError{msg: "ahrefs: --keyword is required"}
			}
			if country == "" {
				return &usageError{msg: "ahrefs: --country is required"}
			}
			q := url.Values{}
			q.Set("country", country)
			q.Set("keyword", keyword)
			if from != "" {
				q.Set("date_from", from)
			}
			if to != "" {
				q.Set("date_to", to)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/keywords-explorer/volume-history", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "the keyword (required)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code (required)")
	cmd.Flags().StringVar(&from, "from", "", "start date YYYY-MM-DD (optional)")
	cmd.Flags().StringVar(&to, "to", "", "end date YYYY-MM-DD (optional)")
	return cmd
}
