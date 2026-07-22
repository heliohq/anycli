package bitly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLinkCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "link", Short: "Bitlinks (shorten, create, expand, get, update, list)"}
	cmd.AddCommand(
		s.newLinkShortenCmd(token),
		s.newLinkCreateCmd(token),
		s.newLinkExpandCmd(token),
		s.newLinkGetCmd(token),
		s.newLinkUpdateCmd(token),
		s.newLinkListCmd(token),
	)
	return cmd
}

func (s *Service) newLinkShortenCmd(token string) *cobra.Command {
	var longURL, domain, group string
	var forceNewLink bool
	cmd := &cobra.Command{
		Use:         "shorten",
		Short:       "Shorten a long URL (POST /shorten)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /shorten creates a bitlink
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			body := map[string]any{
				"long_url":       longURL,
				"domain":         domain,
				"group_guid":     guid,
				"force_new_link": forceNewLink,
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/shorten", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&longURL, "long-url", "", "URL to shorten")
	cmd.Flags().StringVar(&domain, "domain", "bit.ly", "branded short domain")
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().BoolVar(&forceNewLink, "force-new-link", false, "force creation of a new bitlink")
	_ = cmd.MarkFlagRequired("long-url")
	return cmd
}

func (s *Service) newLinkCreateCmd(token string) *cobra.Command {
	var longURL, domain, group, title, keyword, expirationAt, deeplinksJSON string
	var tags []string
	var forceNewLink bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a bitlink with full metadata (POST /bitlinks)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /bitlinks
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			body := map[string]any{
				"long_url":       longURL,
				"domain":         domain,
				"group_guid":     guid,
				"force_new_link": forceNewLink,
			}
			if title != "" {
				body["title"] = title
			}
			if len(tags) > 0 {
				body["tags"] = tags
			}
			if keyword != "" {
				body["keyword"] = keyword
			}
			if expirationAt != "" {
				body["expiration_at"] = expirationAt
			}
			if deeplinksJSON != "" {
				v, err := decodeJSONFlag("deeplinks-json", deeplinksJSON)
				if err != nil {
					return err
				}
				body["deeplinks"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/bitlinks", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&longURL, "long-url", "", "URL to shorten")
	cmd.Flags().StringVar(&domain, "domain", "bit.ly", "branded short domain")
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().StringVar(&title, "title", "", "bitlink title")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag (repeatable)")
	cmd.Flags().StringVar(&keyword, "keyword", "", "custom back-half keyword")
	cmd.Flags().StringVar(&deeplinksJSON, "deeplinks-json", "", "deeplinks JSON array (raw passthrough)")
	cmd.Flags().StringVar(&expirationAt, "expiration-at", "", "ISO-8601 expiration timestamp")
	cmd.Flags().BoolVar(&forceNewLink, "force-new-link", false, "force creation of a new bitlink")
	_ = cmd.MarkFlagRequired("long-url")
	return cmd
}

func (s *Service) newLinkExpandCmd(token string) *cobra.Command {
	var bitlink string
	cmd := &cobra.Command{
		Use:   "expand",
		Short: "Expand a bitlink to its long URL (POST /expand)",
		Args:  cobra.NoArgs,
		// POST /expand is a documented lookup (POST-shaped read); it never
		// mutates provider state under any input.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"bitlink_id": bitlink}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/expand", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bitlink, "bitlink", "", "bitlink id, e.g. bit.ly/2ab")
	_ = cmd.MarkFlagRequired("bitlink")
	return cmd
}

func (s *Service) newLinkGetCmd(token string) *cobra.Command {
	var bitlink string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a bitlink (GET /bitlinks/{bitlink})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/bitlinks/"+bitlink, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bitlink, "bitlink", "", "bitlink id, e.g. bit.ly/2ab (literal slash, not encoded)")
	_ = cmd.MarkFlagRequired("bitlink")
	return cmd
}

func (s *Service) newLinkUpdateCmd(token string) *cobra.Command {
	var bitlink, title, longURL, expirationAt, deeplinksJSON string
	var tags []string
	var archived bool
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a bitlink (PATCH /bitlinks/{bitlink})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PATCH /bitlinks/{bitlink}
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if title != "" {
				body["title"] = title
			}
			if cmd.Flags().Changed("archived") {
				body["archived"] = archived
			}
			if len(tags) > 0 {
				body["tags"] = tags
			}
			if longURL != "" {
				body["long_url"] = longURL
			}
			if expirationAt != "" {
				body["expiration_at"] = expirationAt
			}
			if deeplinksJSON != "" {
				v, err := decodeJSONFlag("deeplinks-json", deeplinksJSON)
				if err != nil {
					return err
				}
				body["deeplinks"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/bitlinks/"+bitlink, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bitlink, "bitlink", "", "bitlink id, e.g. bit.ly/2ab")
	cmd.Flags().StringVar(&title, "title", "", "bitlink title")
	cmd.Flags().BoolVar(&archived, "archived", false, "archive state")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag (repeatable)")
	cmd.Flags().StringVar(&longURL, "long-url", "", "destination long URL")
	cmd.Flags().StringVar(&expirationAt, "expiration-at", "", "ISO-8601 expiration timestamp")
	cmd.Flags().StringVar(&deeplinksJSON, "deeplinks-json", "", "deeplinks JSON array (raw passthrough)")
	_ = cmd.MarkFlagRequired("bitlink")
	return cmd
}

func (s *Service) newLinkListCmd(token string) *cobra.Command {
	var group, searchAfter, query, createdBefore, createdAfter, archived, campaignGUID, channelGUID string
	var tags []string
	var size int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a group's bitlinks (GET /groups/{group_guid}/bitlinks)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("size", intToString(size))
			q.Set("archived", archived)
			if searchAfter != "" {
				q.Set("search_after", searchAfter)
			}
			if query != "" {
				q.Set("query", query)
			}
			if createdBefore != "" {
				q.Set("created_before", createdBefore)
			}
			if createdAfter != "" {
				q.Set("created_after", createdAfter)
			}
			if campaignGUID != "" {
				q.Set("campaign_guid", campaignGUID)
			}
			if channelGUID != "" {
				q.Set("channel_guid", channelGUID)
			}
			for _, tag := range tags {
				q.Add("tags", tag)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups/"+url.PathEscape(guid)+"/bitlinks", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().IntVar(&size, "size", 50, "page size")
	cmd.Flags().StringVar(&searchAfter, "search-after", "", "pagination cursor")
	cmd.Flags().StringVar(&query, "query", "", "keyword search")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "created-before unix timestamp")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "created-after unix timestamp")
	cmd.Flags().StringVar(&archived, "archived", "off", "archived filter: on|off|both")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag filter (repeatable)")
	cmd.Flags().StringVar(&campaignGUID, "campaign-guid", "", "filter by campaign guid")
	cmd.Flags().StringVar(&channelGUID, "channel-guid", "", "filter by channel guid")
	return cmd
}
