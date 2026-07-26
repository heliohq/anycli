package metaads

import "github.com/spf13/cobra"

const defaultAdSetFields = "id,name,campaign_id,status,effective_status,daily_budget,lifetime_budget,billing_event,optimization_goal,bid_amount,start_time,end_time,targeting"

func (s *Service) newAdSetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "adset", Short: "Ad sets"}
	cmd.AddCommand(
		s.newAdSetListCmd(token),
		s.newAdSetGetCmd(token),
		s.newAdSetUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newAdSetListCmd(token string) *cobra.Command {
	var flags edgeListFlags
	var campaign string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ad sets in an ad account (GET /act_<id>/adsets)",
		Long: "--account act_<id> is required; --campaign narrows to one campaign's ad\n" +
			"sets and must be a bare numeric id. --limit is 1-500, default 50, paging\n" +
			"with --after. The default fields include `targeting`, `optimization_goal`\n" +
			"and both budget fields — the ad set is where an ad's actual spend\n" +
			"behaviour lives, and the campaign above it usually carries none of that.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireOptionalObjectID("--campaign", campaign); err != nil {
				return err
			}
			extra := map[string]string{}
			if campaign != "" {
				extra["campaign_id"] = campaign
			}
			return s.listEdge(cmd, token, "adsets", &flags, extra)
		},
	}
	flags.bind(cmd, defaultAdSetFields)
	cmd.Flags().StringVar(&campaign, "campaign", "", "filter ad sets by campaign id")
	return cmd
}

func (s *Service) newAdSetGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <adset_id>",
		Short: "Get one ad set",
		Long: "Takes a bare numeric ad set id. This is the level that owns budget,\n" +
			"schedule, bid and targeting, so it is the object to read when the question\n" +
			"is who an ad reaches or what it is allowed to spend. --fields defaults to\n" +
			"the curated set; widen it for anything Graph does not return by default.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getObject(cmd, token, "adset id", args[0], fields)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultAdSetFields, "comma-separated fields to return")
	return cmd
}

func (s *Service) newAdSetUpdateCmd(token string) *cobra.Command {
	form := updateForm{}
	var name string
	cmd := &cobra.Command{
		Use:   "update <adset_id>",
		Short: "Update an ad set's status, budget, or name (POST /<adset_id>)",
		Long: "Takes a bare numeric ad set id and needs at least one of --status,\n" +
			"--daily-budget, --lifetime-budget or --name. Budgets are INTEGERS in the\n" +
			"ad account currency's minor unit — 5000 is 50.00 in USD. Setting --status\n" +
			"ACTIVE starts spend immediately, but only takes real effect while the\n" +
			"campaign above it is also active.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return form.run(s, cmd, token, "adset id", args[0], name)
		},
	}
	form.bind(cmd)
	cmd.Flags().StringVar(&name, "name", "", "new ad set name")
	return cmd
}
