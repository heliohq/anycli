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
		Use:         "list",
		Short:       "List ads in an ad account (GET /act_<id>/ads)",
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
		Use:         "get <ad_id>",
		Short:       "Get one ad",
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
		Use:         "update <ad_id>",
		Short:       "Update an ad's status or name (POST /<ad_id>)",
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
