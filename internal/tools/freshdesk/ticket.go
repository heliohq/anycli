package freshdesk

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newTicketCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{Use: "ticket", Short: "Tickets (list, get, create, update, search, reply, note, conversations)"}
	cmd.AddCommand(
		s.newTicketListCmd(c),
		s.newTicketGetCmd(c),
		s.newTicketCreateCmd(c),
		s.newTicketUpdateCmd(c),
		s.newTicketSearchCmd(c),
		s.newTicketReplyCmd(c),
		s.newTicketNoteCmd(c),
		s.newTicketConversationsCmd(c),
	)
	return cmd
}

func (s *Service) newTicketListCmd(c *client) *cobra.Command {
	var filter, requesterID, email, companyID, updatedSince, orderBy, orderType string
	var include []string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tickets (GET /tickets). Use `ticket search` to filter by status/priority.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "filter", filter)
			setNonEmpty(q, "requester_id", requesterID)
			setNonEmpty(q, "email", email)
			setNonEmpty(q, "company_id", companyID)
			setNonEmpty(q, "updated_since", updatedSince)
			setNonEmpty(q, "order_by", orderBy)
			setNonEmpty(q, "order_type", orderType)
			if len(include) > 0 {
				q.Set("include", joinCSV(include))
			}
			applyPaging(q, page, perPage)
			resp, err := c.call(cmd.Context(), http.MethodGet, "/tickets", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "predefined filter: new_and_my_open|watching|spam|deleted")
	cmd.Flags().StringVar(&requesterID, "requester-id", "", "filter by requester id")
	cmd.Flags().StringVar(&email, "email", "", "filter by requester email")
	cmd.Flags().StringVar(&companyID, "company-id", "", "filter by company id")
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "ISO-8601 updated-since timestamp")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "order by: created_at|due_by|updated_at|status")
	cmd.Flags().StringVar(&orderType, "order-type", "", "order direction: asc|desc")
	cmd.Flags().StringSliceVar(&include, "include", nil, "embed extra data: stats|requester|description (repeatable/CSV)")
	registerPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newTicketGetCmd(c *client) *cobra.Command {
	var id string
	var include []string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a ticket (GET /tickets/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if len(include) > 0 {
				q.Set("include", joinCSV(include))
			}
			resp, err := c.call(cmd.Context(), http.MethodGet, "/tickets/"+url.PathEscape(id), q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringSliceVar(&include, "include", nil, "embed extra data: conversations|requester|company|stats (repeatable/CSV)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTicketCreateCmd(c *client) *cobra.Command {
	var subject, description, email, priority, status, groupID, responderID, requesterID, customFieldsJSON string
	var tags, cc []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket (POST /tickets)",
		Long: "Create a ticket (POST /tickets).\n\n" +
			"The only field the API requires is a requester identifier — pass\n" +
			"--email or --requester-id. Status, priority, subject, and description\n" +
			"are optional: Freshdesk defaults status to 2 (Open) and priority to 1\n" +
			"(Low) when they are omitted. Passing explicit --status and --priority\n" +
			"is still good practice so the ticket lands in the state you intend.\n" +
			"Status: 2 Open|3 Pending|4 Resolved|5 Closed. Priority: 1 Low|2\n" +
			"Medium|3 High|4 Urgent.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyStr(body, "subject", subject)
			setBodyStr(body, "description", description)
			setBodyStr(body, "email", email)
			setBodyInt(body, "requester_id", requesterID)
			setBodyInt(body, "priority", priority)
			setBodyInt(body, "status", status)
			setBodyInt(body, "group_id", groupID)
			setBodyInt(body, "responder_id", responderID)
			if len(tags) > 0 {
				body["tags"] = tags
			}
			if len(cc) > 0 {
				body["cc_emails"] = cc
			}
			if err := applyCustomFields(body, customFieldsJSON); err != nil {
				return err
			}
			resp, err := c.call(cmd.Context(), http.MethodPost, "/tickets", nil, body)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "ticket subject")
	cmd.Flags().StringVar(&description, "description", "", "ticket description (HTML)")
	cmd.Flags().StringVar(&email, "email", "", "requester email (use this or --requester-id)")
	cmd.Flags().StringVar(&requesterID, "requester-id", "", "requester contact id (use this or --email)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: 1 Low|2 Medium|3 High|4 Urgent")
	cmd.Flags().StringVar(&status, "status", "", "status: 2 Open|3 Pending|4 Resolved|5 Closed")
	cmd.Flags().StringVar(&groupID, "group-id", "", "assign to group id")
	cmd.Flags().StringVar(&responderID, "responder-id", "", "assign to agent id")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "tag (repeatable/CSV)")
	cmd.Flags().StringSliceVar(&cc, "cc", nil, "cc email (repeatable/CSV)")
	cmd.Flags().StringVar(&customFieldsJSON, "custom-fields", "", "custom fields JSON object (raw passthrough)")
	return cmd
}

func (s *Service) newTicketUpdateCmd(c *client) *cobra.Command {
	var id, subject, description, priority, status, groupID, responderID, customFieldsJSON string
	var tags []string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a ticket (PUT /tickets/{id})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyStr(body, "subject", subject)
			setBodyStr(body, "description", description)
			setBodyInt(body, "priority", priority)
			setBodyInt(body, "status", status)
			setBodyInt(body, "group_id", groupID)
			setBodyInt(body, "responder_id", responderID)
			// Freshdesk replaces the tag set on update; --tags is the full
			// desired set (no client-side merge).
			if cmd.Flags().Changed("tags") {
				body["tags"] = tags
			}
			if err := applyCustomFields(body, customFieldsJSON); err != nil {
				return err
			}
			resp, err := c.call(cmd.Context(), http.MethodPut, "/tickets/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringVar(&subject, "subject", "", "ticket subject")
	cmd.Flags().StringVar(&description, "description", "", "ticket description (HTML)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: 1 Low|2 Medium|3 High|4 Urgent")
	cmd.Flags().StringVar(&status, "status", "", "status: 2 Open|3 Pending|4 Resolved|5 Closed")
	cmd.Flags().StringVar(&groupID, "group-id", "", "assign to group id")
	cmd.Flags().StringVar(&responderID, "responder-id", "", "assign to agent id")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "full replacement tag set (repeatable/CSV)")
	cmd.Flags().StringVar(&customFieldsJSON, "custom-fields", "", "custom fields JSON object (raw passthrough)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTicketSearchCmd(c *client) *cobra.Command {
	var query string
	var page int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search tickets (GET /search/tickets). --query is Freshdesk query syntax, e.g. status:2 AND priority:4",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("query", quoteQuery(query))
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			resp, err := c.call(cmd.Context(), http.MethodGet, "/search/tickets", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Freshdesk query, e.g. \"status:2 AND priority:4\"")
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-10; search is capped at 10 pages)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func (s *Service) newTicketReplyCmd(c *client) *cobra.Command {
	var id, body string
	var cc, bcc []string
	cmd := &cobra.Command{
		Use:         "reply",
		Short:       "Reply to a ticket, visible to the requester (POST /tickets/{id}/reply)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"body": body}
			if len(cc) > 0 {
				payload["cc_emails"] = cc
			}
			if len(bcc) > 0 {
				payload["bcc_emails"] = bcc
			}
			resp, err := c.call(cmd.Context(), http.MethodPost, "/tickets/"+url.PathEscape(id)+"/reply", nil, payload)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringVar(&body, "body", "", "reply body (HTML)")
	cmd.Flags().StringSliceVar(&cc, "cc", nil, "cc email (repeatable/CSV)")
	cmd.Flags().StringSliceVar(&bcc, "bcc", nil, "bcc email (repeatable/CSV)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newTicketNoteCmd(c *client) *cobra.Command {
	var id, body string
	var notify []string
	var private, public bool
	cmd := &cobra.Command{
		Use:         "note",
		Short:       "Add a note to a ticket (POST /tickets/{id}/notes). Notes are private by default.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"body": body}
			// Private by default; --public makes the note visible to the requester.
			payload["private"] = resolvePrivate(cmd, private, public)
			if len(notify) > 0 {
				payload["notify_emails"] = notify
			}
			resp, err := c.call(cmd.Context(), http.MethodPost, "/tickets/"+url.PathEscape(id)+"/notes", nil, payload)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringVar(&body, "body", "", "note body (HTML)")
	cmd.Flags().BoolVar(&private, "private", true, "private note (internal only) — the default")
	cmd.Flags().BoolVar(&public, "public", false, "public note (visible to the requester); takes precedence over --private when both are set")
	cmd.Flags().StringSliceVar(&notify, "notify", nil, "agent email to notify (repeatable/CSV)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newTicketConversationsCmd(c *client) *cobra.Command {
	var id string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "conversations",
		Short:       "List a ticket's conversations (GET /tickets/{id}/conversations)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyPaging(q, page, perPage)
			resp, err := c.call(cmd.Context(), http.MethodGet, "/tickets/"+url.PathEscape(id)+"/conversations", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	registerPagingFlags(cmd, &page, &perPage)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// resolvePrivate resolves the note visibility: private by default, --public
// overrides to a public note.
func resolvePrivate(cmd *cobra.Command, private, public bool) bool {
	if cmd.Flags().Changed("public") && public {
		return false
	}
	return private
}
