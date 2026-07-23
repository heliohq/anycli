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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(1),
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
