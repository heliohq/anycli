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

// longAnalyticsMetric is the per-metric prose, and longAnalyticsPeriod the
// clause every one of them shares. They live next to the shared builder because
// it is the builder that fixes the required --period enum, and the five metrics
// differ only in what they measure.
var longAnalyticsMetric = map[string]string{
	"metrics": "The roll-up counters for the window in a single object. The other four\n" +
		"metrics break the same window down along one axis each, so start here and\n" +
		"reach for them only when the breakdown matters.",
	"visits": "Counts people who OPENED the form, whether or not they answered. Read it\n" +
		"against `analytics submissions` over the same period to reason about\n" +
		"conversion.",
	"submissions": "Submission counts over the window — the aggregate only. The responses\n" +
		"themselves come from `submission list`, which is the sole command that\n" +
		"returns answer content.",
	"drop-off": "Shows where respondents abandon the form. This is the diagnostic for a form\n" +
		"with plenty of visits and few completions: `analytics visits` proves they\n" +
		"arrived, this says where they left.",
	"dimensions": "Splits the window by the dimensions Tally records alongside a response\n" +
		"rather than by time, which is what attributes traffic. `analytics metrics`\n" +
		"holds the totals these break down.",
}

const longAnalyticsPeriod = "\n`--form` and `--period` are both required, and `--period` is validated\n" +
	"locally against today, yesterday, 24h, 7d, 30d, 3m, 6m, 12m and all — a\n" +
	"call without one never reaches Tally."

func (s *Service) newAnalyticsMetricCmd(token, metric string) *cobra.Command {
	var form, period string
	cmd := &cobra.Command{
		Use:         metric,
		Short:       "GET /forms/{formId}/analytics/" + metric,
		Long:        longAnalyticsMetric[metric] + longAnalyticsPeriod,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
