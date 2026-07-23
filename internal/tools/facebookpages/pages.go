package facebookpages

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// defaultPageFields is the profile projection returned by `page get`.
const defaultPageFields = "name,about,category,fan_count,followers_count,link,username"

// newPagesListCmd implements `pages list`: the discovery command an assistant
// runs first. It lists the Pages the connected user manages using the USER
// token. It deliberately does NOT request the per-Page access_token field —
// Page tokens are internal and are resolved per-command via the two-hop, never
// surfaced in discovery output.
func (s *Service) newPagesListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the Pages this user manages (id, name, category, tasks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{"fields": {"id,name,category,tasks"}}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/accounts", query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newPageGetCmd implements `page get --page <page_id>`: read a Page's profile.
// Like every Page-scoped command it resolves the Page token via the two-hop and
// issues the read with that token.
func (s *Service) newPageGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get", Short: "Get a Page's profile", Args: cobra.NoArgs}
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated Graph fields (default: profile summary)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		query := url.Values{"fields": {fieldsOrDefault(fields, defaultPageFields)}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodGet, "/"+url.PathEscape(*pageID), query, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}
