package klaviyo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMetricCmd builds the `metric` group: list/get plus the aggregate query.
func (s *Service) newMetricCmd(token string) *cobra.Command {
	group := newGroupCmd("metric", "Read metrics and run aggregate queries")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List metrics (GET /metrics)", "/metrics", "metric"),
		s.newResourceGetCmd(token, "get", "Get one metric (GET /metrics/{id})", "/metrics/", "metric"),
		s.newMetricAggregateCmd(token),
	)
	return group
}

// newMetricAggregateCmd builds `metric aggregate` → POST /metric-aggregates.
// The aggregation body (metric id, measurements, interval, filters, timezone)
// is provider-shaped and open-ended, so it is supplied verbatim via --data.
func (s *Service) newMetricAggregateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "aggregate",
		Short: "Run a metric aggregate query (POST /metric-aggregates) with a --data JSON:API body",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if data == "" {
				return &usageError{msg: "--data (JSON:API metric-aggregate body) is required"}
			}
			payload, err := parseDataFlag(data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/metric-aggregates", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API metric-aggregate request body (required)")
	return cmd
}
