package klaviyo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newReportCmd builds the `report` group: campaign/flow performance reports.
// --series selects the time-series report; otherwise the aggregated values
// report is used. The report body (statistics, timeframe, conversion metric,
// filters) is provider-shaped and supplied verbatim via --data.
func (s *Service) newReportCmd(token string) *cobra.Command {
	group := newGroupCmd("report", "Run campaign and flow performance reports")
	group.AddCommand(
		s.newReportSubCmd(token, "campaign",
			"Campaign performance report (POST /campaign-values-reports, or --series for /campaign-series-reports)",
			"/campaign-values-reports", "/campaign-series-reports"),
		s.newReportSubCmd(token, "flow",
			"Flow performance report (POST /flow-values-reports, or --series for /flow-series-reports)",
			"/flow-values-reports", "/flow-series-reports"),
	)
	return group
}

// newReportSubCmd builds one report command; --series switches from the values
// endpoint to the series endpoint.
func (s *Service) newReportSubCmd(token, use, short, valuesPath, seriesPath string) *cobra.Command {
	var series bool
	var data string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if data == "" {
				return &usageError{msg: "--data (JSON:API report body) is required"}
			}
			payload, err := parseDataFlag(data)
			if err != nil {
				return err
			}
			path := valuesPath
			if series {
				path = seriesPath
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().BoolVar(&series, "series", false, "run the time-series report instead of the aggregated values report")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API report request body (required)")
	return cmd
}
