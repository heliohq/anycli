package ahrefs

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const serpDefaultSelect = "position,url,title,domain_rating,url_rating,traffic,keywords,backlinks,refdomains"

// newSerpCmd wraps GET /serp-overview/serp-overview: the ranking SERP for a
// keyword+country, one row per position. Requires select+country+keyword.
func (s *Service) newSerpCmd(token string) *cobra.Command {
	var keyword, country, date string
	var topPositions int
	var rf rowFlags
	cmd := &cobra.Command{
		Use:         "serp",
		Short:       "SERP overview for a keyword (GET /serp-overview/serp-overview)",
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
			q.Set("select", rf.selectFields)
			if date != "" {
				q.Set("date", date)
			}
			if topPositions > 0 {
				q.Set("top_positions", strconv.Itoa(topPositions))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/serp-overview/serp-overview", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "the keyword to look up (required)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code (required)")
	cmd.Flags().StringVar(&date, "date", "", "report date YYYY-MM-DD (optional; default latest)")
	cmd.Flags().IntVar(&topPositions, "top-positions", 0, "limit to the top N positions (optional)")
	cmd.Flags().StringVar(&rf.selectFields, "select", serpDefaultSelect, "comma-separated fields to return (unit cost scales with fields)")
	return cmd
}
