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

// newKeywordsCmd builds `keywords organic` — what a site ranks for.
func (s *Service) newKeywordsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "keywords", Short: "Organic keywords a target ranks for"}
	cmd.AddCommand(s.newTargetDateRowsCmd(token, "organic",
		"Organic keywords for a target (GET /site-explorer/organic-keywords)",
		"/site-explorer/organic-keywords", organicKeywordsDefaultSelect, false))
	return cmd
}

// newPagesCmd builds `pages top` — a target's best-performing pages.
func (s *Service) newPagesCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "pages", Short: "Top pages of a target"}
	cmd.AddCommand(s.newTargetDateRowsCmd(token, "top",
		"Best-performing pages for a target (GET /site-explorer/top-pages)",
		"/site-explorer/top-pages", topPagesDefaultSelect, false))
	return cmd
}

// newCompetitorsCmd builds `competitors` — the target's organic competitive set.
// organic-competitors additionally requires --country.
func (s *Service) newCompetitorsCmd(token string) *cobra.Command {
	return s.newTargetDateRowsCmd(token, "competitors",
		"Organic competitors for a target (GET /site-explorer/organic-competitors)",
		"/site-explorer/organic-competitors", competitorsDefaultSelect, true)
}

// newTargetDateRowsCmd builds one Site Explorer rows command that requires
// target+date (and, when countryRequired, country). date defaults to today UTC.
func (s *Service) newTargetDateRowsCmd(token, use, short, path, defaultSelect string, countryRequired bool) *cobra.Command {
	var target, date, country string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
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
