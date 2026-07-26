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
		Long: "Returns the ticket's own state — status, priority, assignee, tags and the\n" +
			"customer it belongs to — and none of its content. The thread the customer\n" +
			"sees is `message list <id>` and the agent-only track is `note list <id>`;\n" +
			"reading a ticket properly means at least two calls.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Spans the whole org, so narrowing is the caller's job: `--query\n" +
			"status=open` and friends are appended to the request untouched, and\n" +
			"`--page` / `--page-size` walk the rest. For one person's tickets use\n" +
			"`customer conversations <id>` instead, which is scoped server-side.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Opens a ticket against an existing customer, so the body has to name that\n" +
			"customer by id — `customer get-by-email` or `customer create` first. This\n" +
			"creates the container only: it sends nothing to the customer, and a\n" +
			"conversation with no `message create` after it is an empty ticket that\n" +
			"agents will see in their queue.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Partial by default: only the fields present in the body change, so\n" +
			"`{\"status\":\"done\"}` closes a ticket without disturbing its priority,\n" +
			"assignee or tags. This is the routing and triage verb — status, priority,\n" +
			"assignee, tags — and it never speaks to the customer; closing a ticket\n" +
			"this way sends them nothing.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
