package ahrefs

import (
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

const (
	organicKeywordsDefaultSelect = "keyword,volume,keyword_difficulty,best_position,best_position_url,sum_traffic,cpc"
	topPagesDefaultSelect        = "url,sum_traffic,keywords,top_keyword,top_keyword_volume"
	competitorsDefaultSelect     = "competitor_domain,domain_rating,keywords_common,traffic,share"
)

// The Long texts for the three Site Explorer rows commands. They sit beside
// the shared builder because it is the builder that fixes the target+date
// contract all three describe, while each call site fixes the endpoint.
const (
	longKeywordsOrganic = "Every organic keyword the target currently ranks for, one row per\n" +
		"keyword with its position, volume, difficulty and estimated traffic.\n" +
		"--target is required and --date defaults to today UTC, so an older\n" +
		"snapshot needs an explicit --date. --country is optional here and slices\n" +
		"the ranking data to one market; without it the rows span every market\n" +
		"Ahrefs tracks. Sorting by traffic is the usual first move: `--order-by\n" +
		"'sum_traffic:desc'`."
	longPagesTop = "The target's best-performing URLs by organic traffic, with the single\n" +
		"keyword driving each one. This is the cheap answer to \"what actually\n" +
		"works on this site\" — one row per page instead of one per keyword, which\n" +
		"`keywords organic` gives. --target is required, --date defaults to today\n" +
		"UTC and --country is optional."
	longCompetitors = "--country is REQUIRED here, unlike the other Site Explorer rows\n" +
		"commands: competition is per market and Ahrefs will not compute it\n" +
		"otherwise. Returns the domains whose organic keywords overlap the\n" +
		"target's, with the common-keyword count and traffic share that quantify\n" +
		"the overlap. --target is required and --date defaults to today UTC."
)

// newKeywordsCmd builds `keywords organic` — what a site ranks for.
func (s *Service) newKeywordsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "keywords", Short: "Organic keywords a target ranks for"}
	cmd.AddCommand(s.newTargetDateRowsCmd(token, "organic",
		"Organic keywords for a target (GET /site-explorer/organic-keywords)",
		longKeywordsOrganic, "/site-explorer/organic-keywords", organicKeywordsDefaultSelect, false))
	return cmd
}

// newPagesCmd builds `pages top` — a target's best-performing pages.
func (s *Service) newPagesCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "pages", Short: "Top pages of a target"}
	cmd.AddCommand(s.newTargetDateRowsCmd(token, "top",
		"Best-performing pages for a target (GET /site-explorer/top-pages)",
		longPagesTop, "/site-explorer/top-pages", topPagesDefaultSelect, false))
	return cmd
}

// newCompetitorsCmd builds `competitors` — the target's organic competitive set.
// organic-competitors additionally requires --country.
func (s *Service) newCompetitorsCmd(token string) *cobra.Command {
	return s.newTargetDateRowsCmd(token, "competitors",
		"Organic competitors for a target (GET /site-explorer/organic-competitors)",
		longCompetitors, "/site-explorer/organic-competitors", competitorsDefaultSelect, true)
}

// newTargetDateRowsCmd builds one Site Explorer rows command that requires
// target+date (and, when countryRequired, country). date defaults to today UTC.
func (s *Service) newTargetDateRowsCmd(token, use, short, long, path, defaultSelect string, countryRequired bool) *cobra.Command {
	var target, date, country string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				return &usageError{msg: "ahrefs: --target is required"}
			}
			if countryRequired && country == "" {
				return &usageError{msg: "ahrefs: --country is required"}
			}
			day := date
			if day == "" {
				day = time.Now().UTC().Format("2006-01-02")
			}
			q := url.Values{}
			q.Set("target", target)
			q.Set("date", day)
			if country != "" {
				q.Set("country", country)
			}
			rf.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "domain or URL to analyze (required)")
	cmd.Flags().StringVar(&date, "date", "", "report date YYYY-MM-DD (default: today UTC)")
	countryHelp := "ISO country code (optional)"
	if countryRequired {
		countryHelp = "ISO country code (required)"
	}
	cmd.Flags().StringVar(&country, "country", "", countryHelp)
	registerRowFlags(cmd, &rf, defaultSelect, true)
	return cmd
}
