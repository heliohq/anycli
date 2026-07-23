package later

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newCampaignsCmd reads campaign performance: GET /v2/campaigns/performance.
// startDate and endDate are required (UTC YYYY-MM-DD, max two-year span);
// every other filter maps to an optional query parameter documented by Later.
func (s *Service) newCampaignsCmd(client *reportingClient) *cobra.Command {
	var (
		startDate, endDate    string
		metrics, instanceIDs  []string
		campaignIDs           []string
		platform, contentType string
		dateBasis             string
		sortProperty          string
		sortDirection         string
		limit                 int
		cursor                string
	)
	cmd := &cobra.Command{
		Use:   "campaigns",
		Short: "Read campaign performance (GET /v2/campaigns/performance)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			query.Set("startDate", startDate)
			query.Set("endDate", endDate)
			for _, m := range metrics {
				query.Add("metrics", m)
			}
			for _, id := range instanceIDs {
				query.Add("instanceIds", id)
			}
			for _, id := range campaignIDs {
				query.Add("campaignIds", id)
			}
			if platform != "" {
				query.Set("platform", platform)
			}
			if contentType != "" {
				query.Set("contentType", contentType)
			}
			if dateBasis != "" {
				query.Set("dateBasis", dateBasis)
			}
			if sortProperty != "" {
				query.Set("sortProperty", sortProperty)
			}
			if sortDirection != "" {
				query.Set("sortDirection", sortDirection)
			}
			if limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			if cursor != "" {
				query.Set("nextCursor", cursor)
			}
			body, err := client.get(cmd.Context(), "/v2/campaigns/performance", query)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&startDate, "start", "", "start date, UTC YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&endDate, "end", "", "end date, UTC YYYY-MM-DD (required)")
	cmd.Flags().StringSliceVar(&metrics, "metrics", nil, "metrics to return, repeatable (e.g. engagements,impressions)")
	cmd.Flags().StringSliceVar(&instanceIDs, "instance-ids", nil, "restrict to these instance ids (default: all accessible)")
	cmd.Flags().StringSliceVar(&campaignIDs, "campaign-ids", nil, "restrict to these campaign ids (max 50)")
	cmd.Flags().StringVar(&platform, "platform", "", "filter by platform (instagram|tiktok|youtube|facebook|…)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "filter by content type (e.g. instagram_reel)")
	cmd.Flags().StringVar(&dateBasis, "date-basis", "", "post_date (default) or performance_date")
	cmd.Flags().StringVar(&sortProperty, "sort", "", "sort property (e.g. estimatedRoi)")
	cmd.Flags().StringVar(&sortDirection, "sort-dir", "", "sort direction: ASC or DESC")
	cmd.Flags().IntVar(&limit, "limit", 0, "max items per page (1-100; omitted = provider default 50)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque nextCursor from a prior response (omit on first page)")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	return cmd
}
