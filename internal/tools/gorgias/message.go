package gorgias

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newMessageCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("message", "Read and post messages on a ticket")
	cmd.AddCommand(
		s.newMessageListCmd(token, base),
		s.newMessageCreateCmd(token, base),
	)
	return cmd
}

func (s *Service) newMessageListCmd(token, base string) *cobra.Command {
	var page pageFlags
	cmd := &cobra.Command{
		Use:   "list <ticket-id>",
		Short: "List a ticket's messages (GET /tickets/{id}/messages)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet,
				"/tickets/"+url.PathEscape(args[0])+"/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	return cmd
}

func (s *Service) newMessageCreateCmd(token, base string) *cobra.Command {
	var body, channel, senderEmail string
	var fromAgent bool
	cmd := &cobra.Command{
		Use:   "create <ticket-id>",
		Short: "Post a reply to a ticket (POST /tickets/{id}/messages)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"channel":    channel,
				"from_agent": fromAgent,
				"body_text":  body,
			}
			if senderEmail != "" {
				payload["sender"] = map[string]any{"email": senderEmail}
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodPost,
				"/tickets/"+url.PathEscape(args[0])+"/messages", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "message body (text)")
	cmd.Flags().StringVar(&channel, "channel", "email", "channel: email|chat|phone|...")
	cmd.Flags().BoolVar(&fromAgent, "from-agent", false, "the message is from an agent")
	cmd.Flags().StringVar(&senderEmail, "sender-email", "", "email of the message sender")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}
