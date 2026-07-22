package instagram

import (
	"net/url"

	"github.com/spf13/cobra"
)

// accountInsightMetrics are the default account-level insight metrics.
// profile_views was deprecated in Graph v22.0 (this service pins v23.0), so the
// default is limited to metrics still valid on the pinned version. Several
// account metrics additionally require metric_type=total_value on v22+/v23,
// which the caller supplies via --metric-type (a passthrough, not defaulted, so
// time_series metrics like follower_count are not forced onto total_value).
const accountInsightMetrics = "reach,follower_count"

func (s *Service) newInsightsCmd(token string) *cobra.Command {
	var (
		metrics    string
		metricType string
		period     string
		since      string
		until      string
	)
	cmd := &cobra.Command{
		Use:   "insights",
		Short: "Account-level insights (GET /me/insights)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("metric", firstNonEmpty(metrics, accountInsightMetrics))
			q.Set("period", firstNonEmpty(period, "day"))
			if metricType != "" {
				q.Set("metric_type", metricType)
			}
			if since != "" {
				q.Set("since", since)
			}
			if until != "" {
				q.Set("until", until)
			}
			body, err := s.get(cmd.Context(), token, "/me/insights", q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated metrics (default: reach,follower_count)")
	cmd.Flags().StringVar(&metricType, "metric-type", "", "metric_type for the request (e.g. total_value, required for several v23 account metrics)")
	cmd.Flags().StringVar(&period, "period", "", "aggregation period (default: day)")
	cmd.Flags().StringVar(&since, "since", "", "range start (Unix timestamp)")
	cmd.Flags().StringVar(&until, "until", "", "range end (Unix timestamp)")
	return cmd
}
