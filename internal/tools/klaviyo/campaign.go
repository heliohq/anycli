package klaviyo

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// longCampaignGet sits here rather than in the generic builder in common.go
// because the builder is resource-agnostic: what it must say is that a
// campaign's content lives on its messages, which is campaign-specific.
const longCampaignGet = "The campaign's configuration and status, but not its content: subject\n" +
	"lines and bodies live on its messages, which is `campaign messages <id>`.\n" +
	"Campaigns are neither created nor edited through this tool."

// newCampaignCmd builds the `campaign` group: list/get/messages/send.
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	group := newGroupCmd("campaign", "Read campaigns and trigger sends")
	group.AddCommand(
		s.newCampaignListCmd(token),
		s.newResourceGetCmd(token, "get", "Get one campaign (GET /campaigns/{id})", longCampaignGet, "/campaigns/", "campaign"),
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
		Long: "Klaviyo REQUIRES a messages.channel filter on this endpoint, surfaced as\n" +
			"--channel with a default of email — so a bare `campaign list` shows email\n" +
			"campaigns only, and sms or mobile_push campaigns are invisible until the\n" +
			"flag names them. A --filter passed alongside is AND-combined with the\n" +
			"channel predicate rather than replacing it, so both constraints apply.\n" +
			"Cursor-paged like every list here.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "A campaign's messages carry the content `campaign get` omits — the channel,\n" +
			"subject, body and template link — so this is the read that answers what a\n" +
			"campaign will actually say. Messages are not editable from here.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Posts a campaign-send-job and answers a job receipt, not a delivery result:\n" +
			"--id names the campaign to send, or --data supplies the whole job body.\n" +
			"The campaign has to already exist and be ready in Klaviyo — this triggers\n" +
			"one, it does not build one. The send goes to the campaign's real audience\n" +
			"and there is no cancel command here. The endpoint may answer with no\n" +
			"body, in which case a local receipt is printed.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
