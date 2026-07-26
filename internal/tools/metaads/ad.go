package metaads

import "github.com/spf13/cobra"

const defaultAdFields = "id,name,adset_id,campaign_id,status,effective_status,creative,created_time,updated_time"

func (s *Service) newAdCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "ad", Short: "Ads"}
	cmd.AddCommand(
		s.newAdListCmd(token),
		s.newAdGetCmd(token),
		s.newAdUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newAdListCmd(token string) *cobra.Command {
	var flags edgeListFlags
	var adset string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ads in an ad account (GET /act_<id>/ads)",
		Long: "--account act_<id> is required; --adset narrows to one ad set's ads and\n" +
			"must be a bare numeric id. --limit is 1-500, default 50, paging with\n" +
			"--after. The default fields carry a `creative` REFERENCE rather than the\n" +
			"creative's content — resolve it through `creative list`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireOptionalObjectID("--adset", adset); err != nil {
				return err
			}
			extra := map[string]string{}
			if adset != "" {
				extra["adset_id"] = adset
			}
			return s.listEdge(cmd, token, "ads", &flags, extra)
		},
	}
	flags.bind(cmd, defaultAdFields)
	cmd.Flags().StringVar(&adset, "adset", "", "filter ads by ad set id")
	return cmd
}

func (s *Service) newAdGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <ad_id>",
		Short: "Get one ad",
		Long: "Takes a bare numeric ad id and returns the ad's status plus the id of its\n" +
			"creative, not the creative itself. `effective_status` is the field worth\n" +
			"reading: an ad configured ACTIVE can still be effectively paused by the ad\n" +
			"set or campaign above it, or held in review.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getObject(cmd, token, "ad id", args[0], fields)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultAdFields, "comma-separated fields to return")
	return cmd
}

func (s *Service) newAdUpdateCmd(token string) *cobra.Command {
	form := updateForm{}
	var name string
	cmd := &cobra.Command{
		Use:   "update <ad_id>",
		Short: "Update an ad's status or name (POST /<ad_id>)",
		Long: "Takes a bare numeric ad id and needs at least one of --status or --name.\n" +
			"--daily-budget and --lifetime-budget are accepted here because the update\n" +
			"form is shared with campaigns and ad sets, but budget does NOT live on an\n" +
			"ad — set it with `adset update`. --status is ACTIVE, PAUSED or ARCHIVED.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return form.run(s, cmd, token, "ad id", args[0], name)
		},
	}
	form.bind(cmd)
	cmd.Flags().StringVar(&name, "name", "", "new ad name")
	return cmd
}
