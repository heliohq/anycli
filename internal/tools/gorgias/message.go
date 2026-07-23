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
		Use:         "list <ticket-id>",
		Short:       "List a ticket's messages (GET /tickets/{id}/messages)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
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
	var body, channel, via, senderEmail, sourceFrom string
	var sourceTo []string
	var fromAgent bool
	cmd := &cobra.Command{
		Use:         "create <ticket-id>",
		Short:       "Post a reply to a ticket (POST /tickets/{id}/messages)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := buildMessage(messageParams{
				channel:     channel,
				via:         via,
				body:        body,
				fromAgent:   fromAgent,
				senderEmail: senderEmail,
				sourceFrom:  sourceFrom,
				sourceTo:    sourceTo,
			})
			resp, err := s.call(cmd.Context(), token, base, http.MethodPost,
				"/tickets/"+url.PathEscape(args[0])+"/messages", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "message body (text)")
	cmd.Flags().StringVar(&channel, "channel", "api", "channel: api|email|phone|sms|internal-note")
	cmd.Flags().StringVar(&via, "via", "", "delivery via: api|email|internal-note (default: derived from --channel)")
	cmd.Flags().BoolVar(&fromAgent, "from-agent", false, "the message is from an agent")
	cmd.Flags().StringVar(&senderEmail, "sender-email", "", "email of the message sender")
	cmd.Flags().StringVar(&sourceFrom, "source-from", "", "email/phone/sms: sender routing address (email must be a connected Gorgias integration)")
	cmd.Flags().StringArrayVar(&sourceTo, "source-to", nil, "email/phone/sms: recipient routing address (repeatable)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}
