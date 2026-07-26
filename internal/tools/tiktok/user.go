package tiktok

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Creator profile"}
	cmd.AddCommand(s.newUserInfoCmd(token))
	return cmd
}

func (s *Service) newUserInfoCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show the connected TikTok creator",
		Long: "`--fields` defaults to `open_id,union_id,avatar_url,display_name`, which is\n" +
			"what the basic scope covers. `follower_count`, `following_count`,\n" +
			"`likes_count`, `video_count` and `bio_description` must be named\n" +
			"explicitly AND need the profile/stats scopes granted when the account\n" +
			"connected; without them the call fails outright rather than returning the\n" +
			"field empty. `open_id` is the creator's stable id inside this app,\n" +
			"`union_id` the one shared across the developer's apps.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{"fields": {fields}}
			data, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/user/info/", query, nil)
			if err != nil {
				return err
			}
			return s.emitField(data, "user")
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultUserFields, "comma-separated user fields to return")
	return cmd
}
