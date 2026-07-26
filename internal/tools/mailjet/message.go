package mailjet

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageCmd groups sent-message inspection over /v3/REST/message: what was
// sent and its delivery/engagement state.
func (s *Service) newMessageCmd(basic string) *cobra.Command {
	cmd := newGroupCmd("message", "Inspect sent messages (list, get)")
	cmd.AddCommand(
		s.newMessageListCmd(basic),
		s.newMessageGetCmd(basic),
	)
	return cmd
}

func (s *Service) newMessageListCmd(basic string) *cobra.Command {
	var limit, offset int
	var contactID, campaignID int64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sent messages (GET /v3/REST/message)",
		Long: "The delivery record for messages already sent: one row per recipient message,\n" +
			"with its status and event counters. `--contact-id` narrows to one recipient\n" +
			"and `--campaign-id` to one campaign, which is the practical way to answer \"did\n" +
			"this person receive it\". `--limit` defaults to 10, so an unfiltered call shows\n" +
			"only the ten most recent Mailjet returns.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("Limit", itoa(limit))
			q.Set("Offset", itoa(offset))
			if contactID != 0 {
				q.Set("Contact", itoa64(contactID))
			}
			if campaignID != 0 {
				q.Set("Campaign", itoa64(campaignID))
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/message", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "max messages to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	cmd.Flags().Int64Var(&contactID, "contact-id", 0, "filter by recipient contact ID")
	cmd.Flags().Int64Var(&campaignID, "campaign-id", 0, "filter by campaign ID")
	return cmd
}

func (s *Service) newMessageGetCmd(basic string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one message by ID (GET /v3/REST/message/{id})",
		Long: "`--id` is required and is the `MessageID` a `send` response returned for a\n" +
			"specific recipient, not a campaign id. Returns that single message's delivery\n" +
			"state — queued, sent, opened, clicked, bounced, blocked or spam — which is the\n" +
			"authoritative answer for one recipient, where `stat counters` only aggregates.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/message/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "message ID")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
