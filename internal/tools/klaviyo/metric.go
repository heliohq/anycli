package klaviyo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// The metric read Longs, beside this group because the generic builders in
// common.go are resource-agnostic and cannot say what a metric is.
const (
	longMetricList = "Metrics are the event TYPES an account records — \"Placed Order\", \"Opened\n" +
		"Email\" — and not their counts. Their ids are what a `metric aggregate`\n" +
		"body keys on, and their names are what `event create --metric` matches."

	longMetricGet = "One metric's definition and the integration that feeds it. It carries no\n" +
		"numbers at all: occurrences come from `metric aggregate` or `event list`."
)

// newMetricCmd builds the `metric` group: list/get plus the aggregate query.
func (s *Service) newMetricCmd(token string) *cobra.Command {
	group := newGroupCmd("metric", "Read metrics and run aggregate queries")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List metrics (GET /metrics)", longMetricList, "/metrics", "metric"),
		s.newResourceGetCmd(token, "get", "Get one metric (GET /metrics/{id})", longMetricGet, "/metrics/", "metric"),
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
		Long: "--data is required and is the raw JSON:API metric-aggregate body: the\n" +
			"metric id, the measurements, the interval, the timeframe and any filters.\n" +
			"There is no shorthand because the query is open-ended. This is the\n" +
			"counting endpoint — `metric get` returns only a definition — and its\n" +
			"intervals resolve in the account timezone that `account get` reports.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
