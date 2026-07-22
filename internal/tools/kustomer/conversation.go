package kustomer

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newConversationGetCmd: GET /conversations/{id}.
func (s *Service) newConversationGetCmd(base, token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a conversation (ticket) by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/conversations/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
}

// newConversationListCmd: GET /conversations (paginated).
func (s *Service) newConversationListCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List conversations (recent/open tickets)",
		Args:  cobra.NoArgs,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		qs, err := buildQuery(lf.page, lf.pageSize, lf.query)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/conversations"+qs, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newConversationCreateCmd: POST /conversations with a raw JSON body.
func (s *Service) newConversationCreateCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Open a conversation from a JSON body",
		Args:  cobra.NoArgs,
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPost, "/conversations", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newConversationUpdateCmd: PUT /conversations/{id} (status/priority/assignee/tags).
// Kustomer's "Update conversation" endpoint is PUT and applies patch-like
// partial-update semantics by default (only fields present in the body change);
// it does not accept PATCH. A separate PATCH endpoint exists for a different
// purpose (conversation attributes).
func (s *Service) newConversationUpdateCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a conversation from a JSON body",
		Args:  cobra.ExactArgs(1),
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPut, "/conversations/"+url.PathEscape(args[0]), payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
