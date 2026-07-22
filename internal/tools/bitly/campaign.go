package bitly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "campaign", Short: "Campaigns and channels"}
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
		s.newCampaignCreateCmd(token),
		s.newCampaignUpdateCmd(token),
		s.newChannelCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var group string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List campaigns (GET /campaigns)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("group_guid", guid)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	var campaign string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a campaign (GET /campaigns/{campaign_guid})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(campaign), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign guid")
	_ = cmd.MarkFlagRequired("campaign")
	return cmd
}

func (s *Service) newCampaignCreateCmd(token string) *cobra.Command {
	var group, name, description string
	var channelGUIDs []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a campaign (POST /campaigns)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /campaigns
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			body := map[string]any{
				"group_guid": guid,
				"name":       name,
			}
			if description != "" {
				body["description"] = description
			}
			if len(channelGUIDs) > 0 {
				body["channel_guids"] = channelGUIDs
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/campaigns", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "campaign name")
	cmd.Flags().StringVar(&description, "description", "", "campaign description")
	cmd.Flags().StringArrayVar(&channelGUIDs, "channel-guids", nil, "channel guid (repeatable)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newCampaignUpdateCmd(token string) *cobra.Command {
	var campaign, name, description string
	var channelGUIDs []string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a campaign (PATCH /campaigns/{campaign_guid})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PATCH /campaigns/{campaign_guid}
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			if len(channelGUIDs) > 0 {
				body["channel_guids"] = channelGUIDs
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/campaigns/"+url.PathEscape(campaign), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign guid")
	cmd.Flags().StringVar(&name, "name", "", "campaign name")
	cmd.Flags().StringVar(&description, "description", "", "campaign description")
	cmd.Flags().StringArrayVar(&channelGUIDs, "channel-guids", nil, "channel guid (repeatable)")
	_ = cmd.MarkFlagRequired("campaign")
	return cmd
}

func (s *Service) newChannelCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "channel", Short: "Campaign channels"}
	cmd.AddCommand(
		s.newChannelListCmd(token),
		s.newChannelGetCmd(token),
		s.newChannelCreateCmd(token),
		s.newChannelUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newChannelListCmd(token string) *cobra.Command {
	var group string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List channels (GET /channels)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("group_guid", guid)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/channels", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	return cmd
}

func (s *Service) newChannelGetCmd(token string) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a channel (GET /channels/{channel_guid})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/channels/"+url.PathEscape(channel), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel guid")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}

func (s *Service) newChannelCreateCmd(token string) *cobra.Command {
	var group, name, guid string
	var bitlinks, campaigns []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a channel (POST /channels)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /channels
		RunE: func(cmd *cobra.Command, _ []string) error {
			groupGUID, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			body := map[string]any{
				"group_guid": groupGUID,
				"name":       name,
			}
			if guid != "" {
				body["guid"] = guid
			}
			if len(bitlinks) > 0 {
				entries := make([]map[string]any, 0, len(bitlinks))
				for i, bitlink := range bitlinks {
					entry := map[string]any{"bitlink_id": bitlink}
					if i < len(campaigns) && campaigns[i] != "" {
						entry["campaign_guid"] = campaigns[i]
					}
					entries = append(entries, entry)
				}
				body["bitlinks"] = entries
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/channels", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "channel name")
	cmd.Flags().StringArrayVar(&bitlinks, "bitlink", nil, "bitlink id to attach (repeatable)")
	cmd.Flags().StringArrayVar(&campaigns, "campaign", nil, "campaign guid paired positionally with --bitlink (repeatable)")
	cmd.Flags().StringVar(&guid, "guid", "", "client-supplied channel guid (optional)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newChannelUpdateCmd(token string) *cobra.Command {
	var channel, name string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a channel (PATCH /channels/{channel_guid})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PATCH /channels/{channel_guid}
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/channels/"+url.PathEscape(channel), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel guid")
	cmd.Flags().StringVar(&name, "name", "", "channel name")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}
