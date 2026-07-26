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
		Long: "Returns the campaign API identifiers every other campaigns command needs;\n" +
			"there is no lookup by name, so this is the resolution step. --page is\n" +
			"0-INDEXED and the page size is Braze's own — keep advancing until a page\n" +
			"comes back empty. Archived campaigns are hidden unless --include-archived\n" +
			"is passed, which is why a campaign that exists in the dashboard can be\n" +
			"missing here.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--campaign-id is required and is the API identifier from `campaigns list`,\n" +
			"not the name shown in the dashboard. Returns how the campaign is\n" +
			"configured — channels, messages, schedule — and not how it performed,\n" +
			"which is `campaigns series`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--campaign-id is required. --length is days back from --ending-at (default\n" +
			"now, --length default 7) and caps at 100 — unlike the Canvas series, which\n" +
			"caps at 14. Numbers are aggregated across the whole campaign; a single\n" +
			"tracked send is broken out by `sends series` instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Only works on a campaign the dashboard marked as API-triggered; an ordinary\n" +
			"scheduled campaign cannot be fired this way. --campaign-id is set into the\n" +
			"body for you and everything else — recipients, trigger_properties,\n" +
			"broadcast, audience — comes from --body as raw Braze JSON. Sending with\n" +
			"`broadcast` and no recipients reaches the campaign's whole audience, which\n" +
			"is not undoable. This endpoint family has a much tighter rate limit than\n" +
			"the account default.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
