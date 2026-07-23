package freshservice

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	// defaultStatus (Open) and defaultPriority (Medium) are supplied on create
	// when the flags are omitted. An API-key create is always an agent creating
	// on behalf of a requester, where these are effectively required; sending
	// the documented portal defaults keeps the primary write path working and
	// both are overridable. See the AI-facing doc for the full enum tables.
	defaultStatus   = 2
	defaultPriority = 2

	// defaultPerPage / maxPerPage bound the standard list endpoints.
	defaultPerPage = 30
	maxPerPage     = 100

	// searchPerPage is the fixed page size GET /tickets/filter always uses;
	// per_page is ignored by that endpoint, so it is a constant, not a flag.
	searchPerPage = 30

	// searchMaxPage is the hard page cap on GET /tickets/filter (30/page × 10
	// pages = 300 results). per_page is ignored by that endpoint.
	searchMaxPage = 10
)

func (s *Service) newTicketCmd(c *client) *cobra.Command {
	cmd := newResourceGroup("ticket", "Tickets: list, search, read, create, update, reply, note")
	cmd.AddCommand(
		s.newTicketListCmd(c),
		s.newTicketSearchCmd(c),
		s.newTicketGetCmd(c),
		s.newTicketCreateCmd(c),
		s.newTicketUpdateCmd(c),
		s.newTicketReplyCmd(c),
		s.newTicketNoteCmd(c),
	)
	return cmd
}

// newTicketListCmd → GET /tickets. Standard pagination: per_page up to 100,
// page walks the full dataset via the link header.
func (s *Service) newTicketListCmd(c *client) *cobra.Command {
	var updatedSince string
	var perPage, page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tickets (GET /tickets)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePerPage(perPage); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			if updatedSince != "" {
				q.Set("updated_since", updatedSince)
			}
			return s.emitListResult(cmd, c, "/tickets", "tickets", q, page, perPage)
		},
	}
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "only tickets updated at/after this ISO-8601 timestamp")
	cmd.Flags().IntVar(&perPage, "per-page", defaultPerPage, "results per page (max 100)")
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number")
	return cmd
}

// newTicketSearchCmd → GET /tickets/filter. The endpoint fixes 30 results/page
// and ignores per_page, so no --per-page flag is exposed; --page is validated
// to 1–10 (hard cap 300 results). Unlike GET /tickets, the filter endpoint sends
// no Link header — next_page is derived from the body `total` (see
// emitSearchResult).
func (s *Service) newTicketSearchCmd(c *client) *cobra.Command {
	var query string
	var page int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Filter tickets by query (GET /tickets/filter — 30/page fixed, page 1-10)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if page < 1 || page > searchMaxPage {
				return &usageError{msg: fmt.Sprintf("--page must be between 1 and %d for ticket search (GET /tickets/filter caps at 300 results)", searchMaxPage)}
			}
			q := url.Values{}
			// Freshservice expects the filter expression wrapped in double
			// quotes: query="status:2 AND priority:1".
			q.Set("query", `"`+query+`"`)
			q.Set("page", strconv.Itoa(page))
			return s.emitSearchResult(cmd, c, "/tickets/filter", "tickets", q, page, searchMaxPage)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", `filter expression, e.g. "status:2 AND priority:1"`)
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number (1-10)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

// newTicketGetCmd → GET /tickets/{id}. --conversations includes the ticket's
// conversations in the response (?include=conversations).
func (s *Service) newTicketGetCmd(c *client) *cobra.Command {
	var withConversations bool
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one ticket (GET /tickets/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if withConversations {
				q.Set("include", "conversations")
			}
			body, _, err := c.call(cmd.Context(), http.MethodGet, "/tickets/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emitResource(body, "ticket")
		},
	}
	cmd.Flags().BoolVar(&withConversations, "conversations", false, "include the ticket's conversations")
	return cmd
}

// newTicketCreateCmd → POST /tickets. subject/description/email are required;
// status/priority default to Open/Medium when omitted (see defaultStatus).
func (s *Service) newTicketCreateCmd(c *client) *cobra.Command {
	var subject, description, email, ticketType string
	var status, priority, groupID, agentID int
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a ticket (POST /tickets)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"subject":     subject,
				"description": description,
				"email":       email,
				"status":      status,
				"priority":    priority,
			}
			if groupID != 0 {
				body["group_id"] = groupID
			}
			if agentID != 0 {
				body["responder_id"] = agentID
			}
			if ticketType != "" {
				body["type"] = ticketType
			}
			resp, _, err := c.call(cmd.Context(), http.MethodPost, "/tickets", nil, body)
			if err != nil {
				return err
			}
			return s.emitResource(resp, "ticket")
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "ticket subject")
	cmd.Flags().StringVar(&description, "description", "", "ticket description (HTML allowed)")
	cmd.Flags().StringVar(&email, "email", "", "requester email (the employee raising the ticket)")
	cmd.Flags().IntVar(&status, "status", defaultStatus, "status code (2 Open, 3 Pending, 4 Resolved, 5 Closed)")
	cmd.Flags().IntVar(&priority, "priority", defaultPriority, "priority code (1 Low, 2 Medium, 3 High, 4 Urgent)")
	cmd.Flags().IntVar(&groupID, "group-id", 0, "assignment group id")
	cmd.Flags().IntVar(&agentID, "agent-id", 0, "assigned agent id (responder_id); omit to leave unassigned")
	cmd.Flags().StringVar(&ticketType, "type", "", `ticket type string, e.g. "Incident" or "Service Request"`)
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("description")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

// newTicketUpdateCmd → PUT /tickets/{id}. Only flags the caller changed are
// sent, so an update never clobbers unspecified fields.
func (s *Service) newTicketUpdateCmd(c *client) *cobra.Command {
	var status, priority, groupID, agentID int
	var tags []string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a ticket (PUT /tickets/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if cmd.Flags().Changed("status") {
				body["status"] = status
			}
			if cmd.Flags().Changed("priority") {
				body["priority"] = priority
			}
			if cmd.Flags().Changed("group-id") {
				body["group_id"] = groupID
			}
			if cmd.Flags().Changed("agent-id") {
				body["responder_id"] = agentID
			}
			if cmd.Flags().Changed("tags") {
				body["tags"] = tags
			}
			if len(body) == 0 {
				return &usageError{msg: "ticket update needs at least one of --status, --priority, --group-id, --agent-id, --tags"}
			}
			resp, _, err := c.call(cmd.Context(), http.MethodPut, "/tickets/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emitResource(resp, "ticket")
		},
	}
	cmd.Flags().IntVar(&status, "status", 0, "status code (2 Open, 3 Pending, 4 Resolved, 5 Closed)")
	cmd.Flags().IntVar(&priority, "priority", 0, "priority code (1 Low, 2 Medium, 3 High, 4 Urgent)")
	cmd.Flags().IntVar(&groupID, "group-id", 0, "assignment group id")
	cmd.Flags().IntVar(&agentID, "agent-id", 0, "assigned agent id (responder_id)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "replace the ticket's tags (comma-separated)")
	return cmd
}

// newTicketReplyCmd → POST /tickets/{id}/reply (public reply to the requester).
func (s *Service) newTicketReplyCmd(c *client) *cobra.Command {
	var replyBody string
	cmd := &cobra.Command{
		Use:         "reply <id>",
		Short:       "Reply to the requester (POST /tickets/{id}/reply — public)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, _, err := c.call(cmd.Context(), http.MethodPost, "/tickets/"+url.PathEscape(args[0])+"/reply", nil, map[string]any{"body": replyBody})
			if err != nil {
				return err
			}
			return s.emitResource(resp, "conversation")
		},
	}
	cmd.Flags().StringVar(&replyBody, "body", "", "reply body (HTML allowed)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// newTicketNoteCmd → POST /tickets/{id}/notes. Notes are private by default;
// --private=false posts a public note.
func (s *Service) newTicketNoteCmd(c *client) *cobra.Command {
	var noteBody string
	var private bool
	cmd := &cobra.Command{
		Use:         "note <id>",
		Short:       "Add a note (POST /tickets/{id}/notes — private by default)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, _, err := c.call(cmd.Context(), http.MethodPost, "/tickets/"+url.PathEscape(args[0])+"/notes", nil, map[string]any{
				"body":    noteBody,
				"private": private,
			})
			if err != nil {
				return err
			}
			return s.emitResource(resp, "conversation")
		},
	}
	cmd.Flags().StringVar(&noteBody, "body", "", "note body (HTML allowed)")
	cmd.Flags().BoolVar(&private, "private", true, "private note (default true); --private=false posts a public note")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// validatePerPage rejects an out-of-range per_page before the request, matching
// Freshservice's own 1–100 rule with a fail-fast usage error.
func validatePerPage(perPage int) error {
	if perPage < 1 || perPage > maxPerPage {
		return &usageError{msg: fmt.Sprintf("--per-page must be between 1 and %d", maxPerPage)}
	}
	return nil
}
