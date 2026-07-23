package klaviyo

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCampaignCmd builds the `campaign` group: list/get/messages/send.
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	group := newGroupCmd("campaign", "Read campaigns and trigger sends")
	group.AddCommand(
		s.newCampaignListCmd(token),
		s.newResourceGetCmd(token, "get", "Get one campaign (GET /campaigns/{id})", "/campaigns/", "campaign"),
		s.newCampaignMessagesCmd(token),
		s.newCampaignSendCmd(token),
	)
	return group
}

// newCampaignListCmd builds `campaign list`. Klaviyo requires a
// messages.channel filter on GET /campaigns, surfaced as --channel (default
// email). A user-supplied --filter is AND-combined with the required channel
// predicate so both constraints apply.
func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	f := &listFlags{}
	var channel string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns), required channel via --channel email|sms|mobile_push",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			channelFilter, err := campaignChannelFilter(channel)
			if err != nil {
				return err
			}
			// Build the base query from shared flags, then override filter with the
			// required (and possibly AND-combined) channel predicate.
			userFilter := f.filter
			f.filter = ""
			q, err := f.query("campaign")
			if err != nil {
				return err
			}
			if userFilter != "" {
				q.Set("filter", fmt.Sprintf("and(%s,%s)", channelFilter, userFilter))
			} else {
				q.Set("filter", channelFilter)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	cmd.Flags().StringVar(&channel, "channel", "email", "required message channel: email|sms|mobile_push")
	return cmd
}

// campaignChannelFilter returns the required messages.channel equals predicate.
func campaignChannelFilter(channel string) (string, error) {
	switch channel {
	case "email", "sms", "mobile_push":
		return fmt.Sprintf("equals(messages.channel,'%s')", channel), nil
	default:
		return "", &usageError{msg: "--channel must be email, sms, or mobile_push, got " + channel}
	}
}

func (s *Service) newCampaignMessagesCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:   "messages <id>",
		Short: "List a campaign's messages (GET /campaigns/{id}/campaign-messages)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := f.query("campaign-message")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(args[0])+"/campaign-messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}

// newCampaignSendCmd builds `campaign send` → POST /campaign-send-jobs, which
// takes a campaign-send-job resource identified by the campaign id.
func (s *Service) newCampaignSendCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Trigger a campaign send (POST /campaign-send-jobs) via --id or --data",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var payload any
			if data != "" {
				var err error
				if payload, err = parseDataFlag(data); err != nil {
					return err
				}
			} else if id != "" {
				payload = resourceBody("campaign-send-job", id, nil, nil)
			} else {
				return &usageError{msg: "provide --id (campaign id), or --data"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/campaign-send-jobs", nil, payload)
			if err != nil {
				return err
			}
			if len(body) == 0 {
				return s.emit([]byte(`{"status":"ok"}`))
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id to send")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides --id)")
	return cmd
}
