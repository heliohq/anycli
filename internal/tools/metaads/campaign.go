package metaads

import (
	"net/url"

	"github.com/spf13/cobra"
)

const defaultCampaignFields = "id,name,objective,status,effective_status,daily_budget,lifetime_budget,budget_remaining,created_time,updated_time"

func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "campaign", Short: "Campaigns"}
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
		s.newCampaignCreateCmd(token),
		s.newCampaignUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var flags edgeListFlags
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns in an ad account (GET /act_<id>/campaigns)",
		Long: "--account act_<id> is required. --status filters on Meta's EFFECTIVE\n" +
			"status, the computed one, so an object inside a paused parent reads paused\n" +
			"here regardless of how it is configured. --limit is 1-500, default 50, and\n" +
			"--after takes `paging.cursors.after` from the previous page. The default\n" +
			"fields include both budget fields plus `budget_remaining`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			extra := map[string]string{}
			if status != "" {
				// effective_status is a JSON array filter, e.g. ["ACTIVE"].
				extra["effective_status"] = `["` + status + `"]`
			}
			return s.listEdge(cmd, token, "campaigns", &flags, extra)
		},
	}
	flags.bind(cmd, defaultCampaignFields)
	cmd.Flags().StringVar(&status, "status", "", "filter by effective status (e.g. ACTIVE, PAUSED)")
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <campaign_id>",
		Short: "Get one campaign",
		Long: "Takes a bare numeric campaign id — not an `act_` account id — and needs no\n" +
			"--account, since a Graph object node is addressable on its own. --fields\n" +
			"defaults to the same curated set `campaign list` returns; anything else\n" +
			"must be named explicitly, because Graph returns only what is asked for.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getObject(cmd, token, "campaign id", args[0], fields)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultCampaignFields, "comma-separated fields to return")
	return cmd
}

func (s *Service) newCampaignCreateCmd(token string) *cobra.Command {
	var account, name, objective, status, special string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a campaign (POST /act_<id>/campaigns)",
		Long: "--account, --name and --objective are all required, and --objective takes\n" +
			"one of Meta's outcome objectives such as OUTCOME_TRAFFIC, OUTCOME_SALES or\n" +
			"OUTCOME_LEADS. --status defaults to PAUSED so nothing can spend before the\n" +
			"campaign is reviewed. --special-ad-categories is required by Meta on every\n" +
			"campaign create and defaults to an empty JSON array; a campaign about\n" +
			"housing, employment or credit must declare its category here. A campaign\n" +
			"on its own spends nothing — it needs an ad set and an ad beneath it.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireAccountID(account); err != nil {
				return err
			}
			if name == "" {
				return errRequired("--name")
			}
			if objective == "" {
				return errRequired("--objective")
			}
			form := url.Values{
				"name":      {name},
				"objective": {objective},
				"status":    {status},
			}
			// special_ad_categories is required by Meta on campaign create; it
			// defaults to the empty JSON array (no special category).
			form.Set("special_ad_categories", special)
			body, err := s.post(cmd.Context(), token, "/"+account+"/campaigns", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "ad account id in act_<number> form (required)")
	cmd.Flags().StringVar(&name, "name", "", "campaign name (required)")
	cmd.Flags().StringVar(&objective, "objective", "", "campaign objective, e.g. OUTCOME_TRAFFIC (required)")
	cmd.Flags().StringVar(&status, "status", "PAUSED", "initial status (PAUSED or ACTIVE)")
	cmd.Flags().StringVar(&special, "special-ad-categories", "[]", `special ad categories JSON array, e.g. ["HOUSING"]`)
	return cmd
}

func (s *Service) newCampaignUpdateCmd(token string) *cobra.Command {
	form := updateForm{}
	var name string
	cmd := &cobra.Command{
		Use:   "update <campaign_id>",
		Short: "Update a campaign's status, budget, or name (POST /<campaign_id>)",
		Long: "Takes a bare numeric campaign id, and at least one of --status,\n" +
			"--daily-budget, --lifetime-budget or --name must be set. --status is\n" +
			"ACTIVE, PAUSED or ARCHIVED, and moving to ACTIVE releases spend\n" +
			"immediately. Budgets are INTEGERS in the ad account currency's minor unit:\n" +
			"--daily-budget 5000 is 50.00 in a USD account, not 5000 dollars.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return form.run(s, cmd, token, "campaign id", args[0], name)
		},
	}
	form.bind(cmd)
	cmd.Flags().StringVar(&name, "name", "", "new campaign name")
	return cmd
}
