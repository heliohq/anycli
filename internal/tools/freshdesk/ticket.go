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
		Use:   "list",
		Short: "List tickets (GET /tickets). Use `ticket search` to filter by status/priority.",
		Long: "--filter takes Freshdesk's own named views — new_and_my_open, watching,\n" +
			"spam, deleted — and nothing else; there is no status or priority filter on\n" +
			"this endpoint. --include accepts stats, requester and description, and\n" +
			"description folds each ticket's body into the list, saving one\n" +
			"`ticket get` per row. For polling a queue, --updated-since with an\n" +
			"ISO-8601 timestamp is far cheaper than re-reading the whole filter.",
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
		Use:   "get",
		Short: "Get a ticket (GET /tickets/{id})",
		Long: "--include is the reason to reach for this rather than the list:\n" +
			"conversations, requester, company and stats are embedded in the single\n" +
			"response, so `--include conversations,requester` answers what was said and\n" +
			"by whom in one request. Without it the ticket carries ids only. The thread\n" +
			"on its own, paged, is `ticket conversations`.",
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
		Long: "The only field Freshdesk requires is a requester — --email or\n" +
			"--requester-id. Subject, description, status and priority are all\n" +
			"optional, and a create carrying a requester alone succeeds rather than\n" +
			"returning 400: it lands as status 2 (Open), priority 1 (Low). Pass them\n" +
			"explicitly anyway so the ticket arrives in the queue and state intended.\n" +
			"--description is HTML. An --email matching no existing contact creates one\n" +
			"as a side effect, so a typo silently produces a new requester rather than\n" +
			"an error.",
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
		Use:   "update",
		Short: "Update a ticket (PUT /tickets/{id})",
		Long: "--tags REPLACES the entire tag set. There is no merge and no add verb, so\n" +
			"keeping existing tags means reading them with `ticket get` and passing the\n" +
			"full desired list. Every other field is send-only-if-passed, so omitted\n" +
			"flags keep their current values. Resolving or closing is --status 4 or 5;\n" +
			"neither tells the requester anything on its own — only `ticket reply`\n" +
			"emails them.",
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
		Use:   "search",
		Short: "Search tickets (GET /search/tickets). --query is Freshdesk query syntax, e.g. status:2 AND priority:4",
		Long: "--query is Freshdesk's query language, and string values need their own\n" +
			"inner quotes: \"status:2 AND priority:4\", \"requester_id:123\",\n" +
			"\"tag:'billing'\". The outer double quotes the API demands are added when\n" +
			"absent. Results come back 30 to a page with no --per-page to raise it, and\n" +
			"Freshdesk refuses --page above 10, so a query can surface at most about\n" +
			"300 tickets — narrow it rather than paging for more.",
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
		Use:   "reply",
		Short: "Reply to a ticket, visible to the requester (POST /tickets/{id}/reply)",
		Long: "The reply is EMAILED to the requester the moment this returns. There is no\n" +
			"draft, no scheduling and no recall. --body is HTML. --cc and --bcc apply\n" +
			"to this one message and do not change the ticket's standing cc list. An\n" +
			"internal remark belongs in `ticket note`, which does not mail the\n" +
			"customer.",
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
		Use:   "note",
		Short: "Add a note to a ticket (POST /tickets/{id}/notes). Notes are private by default.",
		Long: "--public is what makes a note visible to the requester, and it wins over\n" +
			"--private when both are passed. --body is HTML. --notify takes agent email\n" +
			"addresses and mails them the note without making it customer-visible,\n" +
			"which is the way to pull a colleague in without touching the customer\n" +
			"thread.",
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
		Use:   "conversations",
		Short: "List a ticket's conversations (GET /tickets/{id}/conversations)",
		Long: "Replies and notes come back interleaved in one list. Each entry carries\n" +
			"`private` (true for an internal note) and `incoming` (true when the\n" +
			"customer wrote it); that pair is the only thing separating the customer's\n" +
			"words from the team's. Paged with --page / --per-page, default 30 and max\n" +
			"100. `ticket get --include conversations` returns the same thread in one\n" +
			"call when the ticket fields are wanted too.",
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
