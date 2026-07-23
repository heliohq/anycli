package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := newGroupCmd("campaign", "Campaigns (list, get, create, update, start/stop, analytics)")
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
		s.newCampaignCreateCmd(token),
		s.newCampaignUpdateCmd(token),
		s.newCampaignActivateCmd(token),
		s.newCampaignPauseCmd(token),
		s.newCampaignSendingStatusCmd(token),
		s.newCampaignAnalyticsCmd(token),
		s.newCampaignAnalyticsOverviewCmd(token),
		s.newCampaignAnalyticsDailyCmd(token),
		s.newCampaignAnalyticsStepsCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var page pageFlags
	var search, status, tagIDs string
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: readOnly,
		Short:       "List campaigns (GET /campaigns)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "search", "search", search)
			setIfChanged(cmd, q, "status", "status", status)
			setIfChanged(cmd, q, "tag-ids", "tag_ids", tagIDs)
			return s.get(cmd, token, "/campaigns", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "filter by name substring")
	cmd.Flags().StringVar(&status, "status", "", "filter by campaign status code")
	cmd.Flags().StringVar(&tagIDs, "tag-ids", "", "comma-separated tag ids")
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a campaign (GET /campaigns/{id})",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/campaigns/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCampaignCreateCmd(token string) *cobra.Command {
	var data, name string
	cmd := &cobra.Command{
		Use:         "create",
		Annotations: writeAction,
		Short:       "Create a campaign (POST /campaigns). --data is the raw JSON body",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("name") {
				body["name"] = name
			}
			return s.send(cmd, token, http.MethodPost, "/campaigns", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON campaign body (sequences, schedule, …)")
	cmd.Flags().StringVar(&name, "name", "", "campaign name (overrides --data.name)")
	return cmd
}

func (s *Service) newCampaignUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:         "update",
		Annotations: writeAction,
		Short:       "Update a campaign (PATCH /campaigns/{id}). --data is the raw JSON body",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			return s.send(cmd, token, http.MethodPatch, "/campaigns/"+url.PathEscape(id), body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON patch body")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCampaignActivateCmd(token string) *cobra.Command {
	return s.campaignAction(token, "activate", "Activate a campaign (POST /campaigns/{id}/activate)", "/activate")
}

func (s *Service) newCampaignPauseCmd(token string) *cobra.Command {
	return s.campaignAction(token, "pause", "Pause a campaign (POST /campaigns/{id}/pause)", "/pause")
}

// campaignAction builds a no-body POST action on a single campaign id.
func (s *Service) campaignAction(token, use, short, suffix string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         use,
		Annotations: writeAction,
		Short:       short,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodPost, "/campaigns/"+url.PathEscape(id)+suffix, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCampaignSendingStatusCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "sending-status",
		Annotations: readOnly,
		Short:       "Get a campaign's sending status (GET /campaigns/{id}/sending-status)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/campaigns/"+url.PathEscape(id)+"/sending-status", nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCampaignAnalyticsCmd(token string) *cobra.Command {
	var id, ids, startDate, endDate string
	cmd := &cobra.Command{
		Use:         "analytics",
		Annotations: readOnly,
		Short:       "Campaign analytics (GET /campaigns/analytics)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd, q, "id", "id", id)
			setIfChanged(cmd, q, "ids", "ids", ids)
			setIfChanged(cmd, q, "start-date", "start_date", startDate)
			setIfChanged(cmd, q, "end-date", "end_date", endDate)
			return s.get(cmd, token, "/campaigns/analytics", q)
		},
	}
	registerAnalyticsRangeFlags(cmd, &startDate, &endDate)
	cmd.Flags().StringVar(&id, "id", "", "single campaign id")
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated campaign ids")
	return cmd
}

func (s *Service) newCampaignAnalyticsOverviewCmd(token string) *cobra.Command {
	var id, ids, startDate, endDate string
	cmd := &cobra.Command{
		Use:         "analytics-overview",
		Annotations: readOnly,
		Short:       "Aggregate campaign analytics (GET /campaigns/analytics/overview)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd, q, "id", "id", id)
			setIfChanged(cmd, q, "ids", "ids", ids)
			setIfChanged(cmd, q, "start-date", "start_date", startDate)
			setIfChanged(cmd, q, "end-date", "end_date", endDate)
			return s.get(cmd, token, "/campaigns/analytics/overview", q)
		},
	}
	registerAnalyticsRangeFlags(cmd, &startDate, &endDate)
	cmd.Flags().StringVar(&id, "id", "", "single campaign id")
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated campaign ids")
	return cmd
}

func (s *Service) newCampaignAnalyticsDailyCmd(token string) *cobra.Command {
	var campaignID, startDate, endDate string
	cmd := &cobra.Command{
		Use:         "analytics-daily",
		Annotations: readOnly,
		Short:       "Daily campaign analytics (GET /campaigns/analytics/daily)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd, q, "campaign-id", "campaign_id", campaignID)
			setIfChanged(cmd, q, "start-date", "start_date", startDate)
			setIfChanged(cmd, q, "end-date", "end_date", endDate)
			return s.get(cmd, token, "/campaigns/analytics/daily", q)
		},
	}
	registerAnalyticsRangeFlags(cmd, &startDate, &endDate)
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign id")
	return cmd
}

func (s *Service) newCampaignAnalyticsStepsCmd(token string) *cobra.Command {
	var campaignID, startDate, endDate string
	cmd := &cobra.Command{
		Use:         "analytics-steps",
		Annotations: readOnly,
		Short:       "Per-step campaign analytics (GET /campaigns/analytics/steps)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd, q, "campaign-id", "campaign_id", campaignID)
			setIfChanged(cmd, q, "start-date", "start_date", startDate)
			setIfChanged(cmd, q, "end-date", "end_date", endDate)
			return s.get(cmd, token, "/campaigns/analytics/steps", q)
		},
	}
	registerAnalyticsRangeFlags(cmd, &startDate, &endDate)
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign id")
	return cmd
}

// registerAnalyticsRangeFlags wires the shared --start-date / --end-date window
// flags (YYYY-MM-DD) onto an analytics command.
func registerAnalyticsRangeFlags(cmd *cobra.Command, startDate, endDate *string) {
	cmd.Flags().StringVar(startDate, "start-date", "", "window start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(endDate, "end-date", "", "window end date (YYYY-MM-DD)")
}
