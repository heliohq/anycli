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
			longReportCampaign, "/campaign-values-reports", "/campaign-series-reports"),
		s.newReportSubCmd(token, "flow",
			"Flow performance report (POST /flow-values-reports, or --series for /flow-series-reports)",
			longReportFlow, "/flow-values-reports", "/flow-series-reports"),
	)
	return group
}

// longReportCampaign and longReportFlow are the two report Longs. They sit next
// to the shared builder because it is the builder that fixes the --data-only
// shape and the --series switch, while the resource `type` each body must
// declare differs per report.
const (
	longReportCampaign = "--data is required and is the raw JSON:API report body: statistics,\n" +
		"timeframe, conversion metric and filters. There is no shorthand because\n" +
		"the query is open-ended. --series switches to the time-series endpoint and\n" +
		"changes the resource type the body must declare from\n" +
		"`campaign-values-report` to `campaign-series-report`, so a body written\n" +
		"for one is rejected by the other. Timeframes resolve in the account\n" +
		"timezone `account get` reports."

	longReportFlow = "--data is required and is the raw JSON:API report body: statistics,\n" +
		"timeframe, conversion metric and filters. --series switches to the\n" +
		"time-series endpoint and changes the resource type the body must declare\n" +
		"from `flow-values-report` to `flow-series-report`, so the two bodies are\n" +
		"not interchangeable. Numbers here are per flow and per flow message, which\n" +
		"is the aggregate `metric aggregate` cannot produce."
)

// newReportSubCmd builds one report command; --series switches from the values
// endpoint to the series endpoint.
func (s *Service) newReportSubCmd(token, use, short, long, valuesPath, seriesPath string) *cobra.Command {
	var series bool
	var data string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
