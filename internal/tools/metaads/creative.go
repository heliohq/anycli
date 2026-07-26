package metaads

import "github.com/spf13/cobra"

const defaultCreativeFields = "id,name,object_type,status,thumbnail_url,object_story_spec"

func (s *Service) newCreativeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "creative", Short: "Ad creatives"}
	cmd.AddCommand(s.newCreativeListCmd(token))
	return cmd
}

func (s *Service) newCreativeListCmd(token string) *cobra.Command {
	var flags edgeListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ad creatives in an ad account (GET /act_<id>/adcreatives)",
		Long: "--account act_<id> is required. Creatives are account-level assets that\n" +
			"ads reference by id, so this is how the `creative` id from `ad get`\n" +
			"becomes a name, thumbnail and `object_story_spec`. The group is read-only\n" +
			"— creatives cannot be created or edited here. --limit is 1-500, default\n" +
			"50.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listEdge(cmd, token, "adcreatives", &flags, nil)
		},
	}
	flags.bind(cmd, defaultCreativeFields)
	return cmd
}
