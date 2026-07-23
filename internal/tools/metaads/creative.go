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
		Use:         "list",
		Short:       "List ad creatives in an ad account (GET /act_<id>/adcreatives)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listEdge(cmd, token, "adcreatives", &flags, nil)
		},
	}
	flags.bind(cmd, defaultCreativeFields)
	return cmd
}
