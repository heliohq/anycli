package kustomer

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageListCmd: GET /conversations/{id}/messages (read the thread).
func (s *Service) newMessageListCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <conversation-id>",
		Short: "List a conversation's messages",
		Long: "The customer-facing thread, inbound and outbound alike; internal notes are\n" +
			"NOT in here and come from `note list`. Read this before replying — the\n" +
			"conversation record alone says nothing about what has already been said.\n" +
			"Paged with `--page` / `--page-size`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		qs, err := buildQuery(lf.page, lf.pageSize, lf.query)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/conversations/"+url.PathEscape(args[0])+"/messages"+qs, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newMessageCreateCmd: POST /conversations/{id}/messages (reply to the customer).
func (s *Service) newMessageCreateCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <conversation-id>",
		Short: "Post a message to a conversation from a JSON body",
		Long: "This REACHES THE CUSTOMER: it posts an outbound message on the channel the\n" +
			"conversation runs on, delivered as soon as the call returns, with no draft\n" +
			"state and no edit or delete verb afterwards. Anything meant to stay\n" +
			"internal belongs in `note create` instead. The body is Kustomer's message\n" +
			"shape, sent unmodified, and it decides the channel and direction.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPost, "/conversations/"+url.PathEscape(args[0])+"/messages", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
