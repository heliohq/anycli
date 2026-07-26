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
		Long: "--search matches the campaign NAME as a substring. --status takes\n" +
			"Instantly's numeric campaign status code, not a word like \"active\", and\n" +
			"--tag-ids is comma-separated. Cursor-paged with --limit and\n" +
			"--starting-after. The ids returned here are what every other campaign\n" +
			"command takes.",
		Args: cobra.NoArgs,
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
		Long: "--id is required. Returns the campaign's full configuration — its sequence\n" +
			"steps, schedule and attached sending accounts — which is what a `campaign\n" +
			"update --data` patch has to be built from, since that update has no\n" +
			"per-field flags.",
		Args: cobra.NoArgs,
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
		Long: "--data carries the raw campaign body, because a campaign's sequence steps\n" +
			"and sending schedule are nested structures with no flag equivalent;\n" +
			"--name is a convenience that overrides `name` inside it. Creating a\n" +
			"campaign sends nothing on its own — outreach begins only when `campaign\n" +
			"activate` runs — so building and reviewing one is safe.",
		Args: cobra.NoArgs,
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
		Long: "--id is required and --data is the raw patch body; there are no per-field\n" +
			"flags, so read the current shape with `campaign get` and send back only\n" +
			"the keys that change. Editing a campaign that is currently active changes\n" +
			"what its remaining steps will send, with no separate confirmation.",
		Args: cobra.NoArgs,
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

// longCampaignActivate and longCampaignPause are the two start/stop Longs. They
// sit next to the shared builder because it is the builder that fixes the
// bodyless POST on a single --id both describe, while what the action costs —
// live outreach beginning, or mail already handed off staying gone — is
// opposite in each direction.
const (
	longCampaignActivate = "Takes --id and starts LIVE outreach: the campaign begins sending its\n" +
		"sequence to its leads, to real recipients, as soon as its schedule window\n" +
		"allows. There is no dry run and nothing recalls what goes out. Follow with\n" +
		"`campaign sending-status`, since a campaign with no leads or no attached\n" +
		"sending account activates without sending anything."

	longCampaignPause = "Takes --id and stops the campaign from sending further steps. Mail already\n" +
		"handed to a provider is gone and pausing does not retract it. Leads keep\n" +
		"their position in the sequence, so `campaign activate` resumes from there\n" +
		"rather than restarting from step one."
)

func (s *Service) newCampaignActivateCmd(token string) *cobra.Command {
	return s.campaignAction(token, "activate", "Activate a campaign (POST /campaigns/{id}/activate)", longCampaignActivate, "/activate")
}

func (s *Service) newCampaignPauseCmd(token string) *cobra.Command {
	return s.campaignAction(token, "pause", "Pause a campaign (POST /campaigns/{id}/pause)", longCampaignPause, "/pause")
}

// campaignAction builds a no-body POST action on a single campaign id.
func (s *Service) campaignAction(token, use, short, long, suffix string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         use,
		Annotations: writeAction,
		Short:       short,
		Long:        long,
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
		Long: "--id is required. This answers whether the campaign is actually sending and\n" +
			"what is stopping it if not — an activated campaign still sends nothing\n" +
			"when it has no leads, no attached sending account, or is outside its\n" +
			"schedule window. Check it after `campaign activate` rather than treating\n" +
			"activation as proof of delivery.",
		Args: cobra.NoArgs,
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
		Long: "The headline per-campaign figures — sent, opened, replied, bounced — for a\n" +
			"single --id or a comma-separated --ids in one call. Omitting both covers\n" +
			"every campaign in the workspace. --start-date and --end-date are\n" +
			"YYYY-MM-DD and bound the window; without them the campaign's whole\n" +
			"lifetime is counted.",
		Args: cobra.NoArgs,
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
		Long: "The roll-up ACROSS campaigns, where `campaign analytics` returns a row per\n" +
			"campaign. --id or a comma-separated --ids narrows it; omitting both sums\n" +
			"the whole workspace, which is the one-call answer to \"how is outbound\n" +
			"doing\". --start-date and --end-date are YYYY-MM-DD.",
		Args: cobra.NoArgs,
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
		Long: "The same figures as `campaign analytics`, broken out per DAY — which is how\n" +
			"a trend or a sudden bounce spike becomes visible. The flag here is\n" +
			"--campaign-id, not --id, and it takes one campaign. --start-date and\n" +
			"--end-date are YYYY-MM-DD.",
		Args: cobra.NoArgs,
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
		Long: "Figures per SEQUENCE STEP for one --campaign-id: which email in the\n" +
			"sequence earned the replies and which one drove the unsubscribes. This is\n" +
			"the breakdown `campaign analytics` aggregates away, and the read a\n" +
			"sequence edit should be based on. --start-date and --end-date are\n" +
			"YYYY-MM-DD.",
		Args: cobra.NoArgs,
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
