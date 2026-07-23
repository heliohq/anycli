package sendgrid

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newStatsCmd wraps GET /v3/stats. start_date is required by the API; end_date,
// aggregated_by, and the pagination window are optional.
func (s *Service) newStatsCmd(token string, region *string) *cobra.Command {
	var startDate, endDate, aggregatedBy string
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "stats",
		Short:       "Aggregated email stats (GET /v3/stats)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("start_date", startDate)
			if endDate != "" {
				q.Set("end_date", endDate)
			}
			if aggregatedBy != "" {
				q.Set("aggregated_by", aggregatedBy)
			}
			if limit > 0 {
				q.Set("limit", intToString(limit))
			}
			if offset > 0 {
				q.Set("offset", intToString(offset))
			}
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/stats", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&startDate, "start-date", "", "start date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "end date YYYY-MM-DD (optional)")
	cmd.Flags().StringVar(&aggregatedBy, "aggregated-by", "", "aggregation window: day|week|month")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results (optional)")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset (optional)")
	_ = cmd.MarkFlagRequired("start-date")
	return cmd
}
