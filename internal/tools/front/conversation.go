package front

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newConversationListCmd is `conversation list`. It reads the queue three ways,
// most specific wins: --q routes to the search endpoint
// (GET /conversations/search/{query}); else --inbox lists one inbox's queue
// (GET /inboxes/{id}/conversations); else the whole visible queue
// (GET /conversations). All three share limit / page-token / sort-order.
func (s *Service) newConversationListCmd(token string) *cobra.Command {
	var inbox, q, sortOrder, pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List or search the conversation queue",
		Long: "Three different endpoints hide behind this one verb and the most specific\n" +
			"flag wins: --q routes to Front's search, otherwise --inbox lists a single\n" +
			"inbox's queue, otherwise the whole visible queue comes back. Passing --q\n" +
			"and --inbox together therefore searches everywhere and silently IGNORES\n" +
			"the inbox. --sort-order is asc or desc, with desc putting the most recent\n" +
			"first. --limit is capped at 100; continue with --page-token.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&inbox, "inbox", "", "restrict to one inbox id")
	cmd.Flags().StringVar(&q, "q", "", "search query (routes to the search endpoint)")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "sort order: asc|desc")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Front caps at 100)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "cursor from a prior response's next_page_token")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
			return &usageError{msg: `--sort-order must be one of asc|desc`}
		}
		query := pageQuery(limit, pageToken)
		if sortOrder != "" {
			query.Set("sort_order", sortOrder)
		}
		path := "/conversations"
		switch {
		case q != "":
			path = "/conversations/search/" + url.PathEscape(q)
		case inbox != "":
			path = "/inboxes/" + url.PathEscape(inbox) + "/conversations"
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
		if err != nil {
			return err
		}
		return s.emitList(body)
	}
	return cmd
}

// newConversationGetCmd is `conversation get --id <cnv_id>`
// (GET /conversations/{id}) — one conversation's metadata.
func (s *Service) newConversationGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one conversation's metadata",
		Long: "Metadata only — status, assignee, tags, recipients and the channel it\n" +
			"lives on. Neither the customer thread nor the internal discussion is\n" +
			"included: those are `conversation messages` and `conversation comments`,\n" +
			"so understanding a conversation properly costs three calls. The channel id\n" +
			"in this response is what `draft create --channel` needs.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&id, "id", "", "conversation id (required)")
	_ = cmd.MarkFlagRequired("id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emitObject(body)
	}
	return cmd
}

// newConversationMessagesCmd is `conversation messages --id <cnv_id>`
// (GET /conversations/{id}/messages) — the message thread.
func (s *Service) newConversationMessagesCmd(token string) *cobra.Command {
	var id, pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "messages",
		Short: "List the messages in a conversation",
		Long: "The customer-visible thread: everything sent and received, in Front's own\n" +
			"order. Internal notes are NOT here — they live in `conversation comments`,\n" +
			"a separate list — so reading only this misses whatever colleagues said\n" +
			"about the case. --limit is capped at 100; continue with --page-token.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&id, "id", "", "conversation id (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Front caps at 100)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "cursor from a prior response's next_page_token")
	_ = cmd.MarkFlagRequired("id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet,
			"/conversations/"+url.PathEscape(id)+"/messages", pageQuery(limit, pageToken), nil)
		if err != nil {
			return err
		}
		return s.emitList(body)
	}
	return cmd
}

// newConversationCommentsCmd is `conversation comments --id <cnv_id>`
// (GET /conversations/{id}/comments) — the internal discussion.
func (s *Service) newConversationCommentsCmd(token string) *cobra.Command {
	var id, pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "List the internal comments on a conversation",
		Long: "The teammate-only discussion attached to the conversation, which the\n" +
			"customer never sees. It is a separate list from `conversation messages`,\n" +
			"so a decision explained in a comment will not appear in the thread and\n" +
			"vice versa. --limit is capped at 100; continue with --page-token.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&id, "id", "", "conversation id (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Front caps at 100)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "cursor from a prior response's next_page_token")
	_ = cmd.MarkFlagRequired("id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet,
			"/conversations/"+url.PathEscape(id)+"/comments", pageQuery(limit, pageToken), nil)
		if err != nil {
			return err
		}
		return s.emitList(body)
	}
	return cmd
}

// newConversationUpdateCmd is `conversation update --id <cnv_id>`. It presents
// one triage intent but issues the distinct underlying calls Front requires:
// PATCH /conversations/{id} for status/inbox, PUT …/assignee for assign/unassign,
// and POST/DELETE …/tags for tag add/remove. Each requested change runs in a
// fixed order; the first failure aborts and is returned. On full success it
// reports which operations were applied.
func (s *Service) newConversationUpdateCmd(token string) *cobra.Command {
	var id, status, assignee, inbox string
	var tagAdd, tagRemove []string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Change status, assignee, inbox, or tags on a conversation",
		Long: "One command, but Front needs four different endpoints underneath, and they\n" +
			"run in a fixed order: status and inbox first, then assignee, then tag\n" +
			"additions, then tag removals. It is NOT atomic — a failure part-way leaves\n" +
			"the earlier changes applied, and the success payload's `applied` array is\n" +
			"the record of what actually ran. At least one change flag is required.\n" +
			"\n" +
			"--status is open, archived, deleted or spam; archived is the ordinary\n" +
			"\"done\" and open reopens. --assignee takes a teammate id from `teammate\n" +
			"list`, and the literal `null` or an empty value unassigns. --tag-add and\n" +
			"--tag-remove are repeatable and take tag ids from `tag list`, never tag\n" +
			"names.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&id, "id", "", "conversation id (required)")
	cmd.Flags().StringVar(&status, "status", "", "new status: open|archived|deleted|spam")
	cmd.Flags().StringVar(&assignee, "assignee", "", "teammate id to assign; use 'null' to unassign")
	cmd.Flags().StringVar(&inbox, "inbox", "", "move the conversation to this inbox id")
	cmd.Flags().StringArrayVar(&tagAdd, "tag-add", nil, "tag id to add (repeatable)")
	cmd.Flags().StringArrayVar(&tagRemove, "tag-remove", nil, "tag id to remove (repeatable)")
	_ = cmd.MarkFlagRequired("id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if status != "" && status != "open" && status != "archived" && status != "deleted" && status != "spam" {
			return &usageError{msg: `--status must be one of open|archived|deleted|spam`}
		}
		assigneeSet := cmd.Flags().Changed("assignee")
		if status == "" && inbox == "" && !assigneeSet && len(tagAdd) == 0 && len(tagRemove) == 0 {
			return &usageError{msg: "conversation update requires at least one of --status, --assignee, --inbox, --tag-add, --tag-remove"}
		}
		applied := []string{}
		base := "/conversations/" + url.PathEscape(id)

		if status != "" || inbox != "" {
			patch := map[string]any{}
			if status != "" {
				patch["status"] = status
			}
			if inbox != "" {
				patch["inbox_id"] = inbox
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPatch, base, nil, patch); err != nil {
				return err
			}
			applied = append(applied, "patch")
		}
		if assigneeSet {
			// A literal "null" (or empty) assignee unassigns; Front takes a JSON
			// null for assignee_id.
			var assigneeID any = assignee
			if assignee == "" || assignee == "null" {
				assigneeID = nil
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPut, base+"/assignee", nil,
				map[string]any{"assignee_id": assigneeID}); err != nil {
				return err
			}
			applied = append(applied, "assignee")
		}
		if len(tagAdd) > 0 {
			if _, err := s.call(cmd.Context(), token, http.MethodPost, base+"/tags", nil,
				map[string]any{"tag_ids": tagAdd}); err != nil {
				return err
			}
			applied = append(applied, "tag-add")
		}
		if len(tagRemove) > 0 {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, base+"/tags", nil,
				map[string]any{"tag_ids": tagRemove}); err != nil {
				return err
			}
			applied = append(applied, "tag-remove")
		}
		return s.emitValue(map[string]any{"data": map[string]any{"ok": true, "applied": applied}})
	}
	return cmd
}
