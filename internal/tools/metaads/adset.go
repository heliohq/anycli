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
		Use:         "list",
		Short:       "List ad sets in an ad account (GET /act_<id>/adsets)",
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
		Use:         "get <adset_id>",
		Short:       "Get one ad set",
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
		Use:         "update <adset_id>",
		Short:       "Update an ad set's status, budget, or name (POST /<adset_id>)",
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
