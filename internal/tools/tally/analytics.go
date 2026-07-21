package tally

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// analyticsMetrics are the fixed metric sub-paths under
// /forms/{formId}/analytics/. drop-off carries a dash in the URL segment.
var analyticsMetrics = []string{"metrics", "visits", "submissions", "drop-off", "dimensions"}

// analyticsPeriods is the enum the period query param accepts (required by the
// Tally OpenAPI on every analytics endpoint).
var analyticsPeriods = []string{"today", "yesterday", "24h", "7d", "30d", "3m", "6m", "12m", "all"}

func (s *Service) newAnalyticsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("analytics", "Form analytics (metrics, visits, submissions, drop-off, dimensions)")
	for _, metric := range analyticsMetrics {
		cmd.AddCommand(s.newAnalyticsMetricCmd(token, metric))
	}
	return cmd
}

func (s *Service) newAnalyticsMetricCmd(token, metric string) *cobra.Command {
	var form, period string
	cmd := &cobra.Command{
		Use:   metric,
		Short: "GET /forms/{formId}/analytics/" + metric,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := oneOfFlag("period", period, analyticsPeriods); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("period", period)
			resp, err := s.call(cmd.Context(), token, http.MethodGet,
				"/forms/"+url.PathEscape(form)+"/analytics/"+metric, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	cmd.Flags().StringVar(&period, "period", "", "time window: today|yesterday|24h|7d|30d|3m|6m|12m|all")
	_ = cmd.MarkFlagRequired("form")
	_ = cmd.MarkFlagRequired("period")
	return cmd
}
