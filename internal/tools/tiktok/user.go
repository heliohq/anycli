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
		Args:  cobra.NoArgs,
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
