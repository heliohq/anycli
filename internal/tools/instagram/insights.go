package instagram

import (
	"net/url"

	"github.com/spf13/cobra"
)

// accountInsightMetrics are the default account-level insight metrics.
const accountInsightMetrics = "reach,follower_count,profile_views"

func (s *Service) newInsightsCmd(token string) *cobra.Command {
	var (
		metrics string
		period  string
		since   string
		until   string
	)
	cmd := &cobra.Command{
		Use:   "insights",
		Short: "Account-level insights (GET /me/insights)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("metric", firstNonEmpty(metrics, accountInsightMetrics))
			q.Set("period", firstNonEmpty(period, "day"))
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
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated metrics (default: reach,follower_count,profile_views)")
	cmd.Flags().StringVar(&period, "period", "", "aggregation period (default: day)")
	cmd.Flags().StringVar(&since, "since", "", "range start (Unix timestamp)")
	cmd.Flags().StringVar(&until, "until", "", "range end (Unix timestamp)")
	return cmd
}
