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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return form.run(s, cmd, token, "campaign id", args[0], name)
		},
	}
	form.bind(cmd)
	cmd.Flags().StringVar(&name, "name", "", "new campaign name")
	return cmd
}
