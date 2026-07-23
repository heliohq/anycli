package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newSendsCmd builds the `sends` resource group: send-level analytics for a
// tracked send id (GET /sends/data_series).
func (s *Service) newSendsCmd(c *client) *cobra.Command {
	group := newGroupCmd("sends", "Send-level analytics")
	group.AddCommand(s.newSendsSeriesCmd(c))
	return group
}

// newSendsSeriesCmd is `sends series` (GET /sends/data_series): analytics for a
// specific tracked send within a campaign.
func (s *Service) newSendsSeriesCmd(c *client) *cobra.Command {
	var campaignID, sendID, endingAt string
	var length int
	cmd := &cobra.Command{
		Use:   "series",
		Short: "Get analytics for a tracked send id",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign API identifier (required)")
	cmd.Flags().StringVar(&sendID, "send-id", "", "send identifier from the send-id tracking (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	_ = cmd.MarkFlagRequired("campaign-id")
	_ = cmd.MarkFlagRequired("send-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("campaign_id", campaignID)
		q.Set("send_id", sendID)
		q.Set("length", strconv.Itoa(length))
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		body, err := c.get(cmd.Context(), "/sends/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
