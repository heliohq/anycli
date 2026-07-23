package facebookpages

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// defaultInsightsMetrics is the metric set returned by `insights` when the
// caller does not pass --metrics.
const defaultInsightsMetrics = "page_impressions,page_post_engagements,page_fans"

func (s *Service) newInsightsCmd(token string) *cobra.Command {
	var metrics, period, since, until string
	cmd := &cobra.Command{Use: "insights", Short: "Read Page insights (metrics over a period)", Args: cobra.NoArgs}
	cmd.Annotations = readOnly
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated Graph metrics (default: impressions/engagement/fans)")
	cmd.Flags().StringVar(&period, "period", "day", "aggregation period: day, week, days_28, month, lifetime")
	cmd.Flags().StringVar(&since, "since", "", "range start as a UNIX timestamp")
	cmd.Flags().StringVar(&until, "until", "", "range end as a UNIX timestamp")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		query := url.Values{
			"metric": {fieldsOrDefault(metrics, defaultInsightsMetrics)},
		}
		if strings.TrimSpace(period) != "" {
			query.Set("period", period)
		}
		if since != "" {
			query.Set("since", since)
		}
		if until != "" {
			query.Set("until", until)
		}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodGet, "/"+url.PathEscape(*pageID)+"/insights", query, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}
