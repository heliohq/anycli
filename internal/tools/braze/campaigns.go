package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newCampaignsCmd builds the `campaigns` resource group: list / details /
// series (all GET export) plus trigger (POST act).
func (s *Service) newCampaignsCmd(c *client) *cobra.Command {
	group := newGroupCmd("campaigns", "Campaign inventory, analytics, and API-triggered sends")
	group.AddCommand(
		s.newCampaignsListCmd(c),
		s.newCampaignsDetailsCmd(c),
		s.newCampaignsSeriesCmd(c),
		s.newCampaignsTriggerCmd(c),
	)
	return group
}

// newCampaignsListCmd is `campaigns list` (GET /campaigns/list): the paginated
// campaign inventory (id + name).
func (s *Service) newCampaignsListCmd(c *client) *cobra.Command {
	var page int
	var includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (id + name), paginated",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().IntVar(&page, "page", 0, "0-indexed page of campaigns to return")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "include archived campaigns")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("page") {
			q.Set("page", strconv.Itoa(page))
		}
		if includeArchived {
			q.Set("include_archived", "true")
		}
		body, err := c.get(cmd.Context(), "/campaigns/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCampaignsDetailsCmd is `campaigns details` (GET /campaigns/details).
func (s *Service) newCampaignsDetailsCmd(c *client) *cobra.Command {
	var campaignID string
	cmd := &cobra.Command{
		Use:   "details",
		Short: "Get a campaign's configuration and metadata",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign API identifier (required)")
	_ = cmd.MarkFlagRequired("campaign-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("campaign_id", campaignID)
		body, err := c.get(cmd.Context(), "/campaigns/details", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCampaignsSeriesCmd is `campaigns series` (GET /campaigns/data_series):
// campaign analytics over a window.
func (s *Service) newCampaignsSeriesCmd(c *client) *cobra.Command {
	var campaignID, endingAt string
	var length int
	cmd := &cobra.Command{
		Use:   "series",
		Short: "Get a campaign's analytics time-series",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign API identifier (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	_ = cmd.MarkFlagRequired("campaign-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("campaign_id", campaignID)
		q.Set("length", strconv.Itoa(length))
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		body, err := c.get(cmd.Context(), "/campaigns/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCampaignsTriggerCmd is `campaigns trigger` (POST /campaigns/trigger/send):
// send an API-triggered campaign. The large, versioned recipients /
// trigger_properties body is passed through --body; the tool only sets
// campaign_id. Permission-gated by the REST key's scope.
func (s *Service) newCampaignsTriggerCmd(c *client) *cobra.Command {
	var campaignID, bodyFlag string
	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Send an API-triggered campaign (permission-gated)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign API identifier (required)")
	cmd.Flags().StringVar(&bodyFlag, "body", "", "raw JSON object: recipients, trigger_properties, broadcast, audience, …")
	_ = cmd.MarkFlagRequired("campaign-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := objectBodyFlag("body", bodyFlag, map[string]any{"campaign_id": campaignID})
		if err != nil {
			return err
		}
		body, err := c.post(cmd.Context(), "/campaigns/trigger/send", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
