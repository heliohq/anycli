package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newConversationCmd builds the conversation resource group: the support inbox
// (list/search/get) plus the actions an admin takes on a conversation
// (reply/note/state changes/assignment/tagging).
func (s *Service) newConversationCmd(token string) *cobra.Command {
	cmd := newGroupCmd("conversation", "Inbox conversations: read, search, reply, and act")
	cmd.AddCommand(
		s.newConversationListCmd(token),
		s.newConversationSearchCmd(token),
		s.newConversationGetCmd(token),
		s.newConversationReplyCmd(token),
		s.newConversationNoteCmd(token),
		s.newConversationCloseCmd(token),
		s.newConversationOpenCmd(token),
		s.newConversationSnoozeCmd(token),
		s.newConversationAssignCmd(token),
		s.newConversationTagCmd(token),
		s.newConversationUntagCmd(token),
	)
	return cmd
}

func (s *Service) newConversationListCmd(token string) *cobra.Command {
	var perPage int
	var startingAfter string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List conversations (GET /conversations)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage > 0 {
				q.Set("per_page", intToString(perPage))
			}
			if startingAfter != "" {
				q.Set("starting_after", startingAfter)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page (Intercom default 20, max 150)")
	cmd.Flags().StringVar(&startingAfter, "starting-after", "", "pagination cursor from pages.next.starting_after")
	return cmd
}

func (s *Service) newConversationSearchCmd(token string) *cobra.Command {
	var sf searchFlags
	var state, updatedSince string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search conversations (POST /conversations/search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var filters []map[string]any
			if state != "" {
				filters = append(filters, filterEq("state", state))
			}
			if updatedSince != "" {
				filters = append(filters, filterGT("updated_at", updatedSince))
			}
			body, err := buildSearchBody(sf, filters)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerSearchFlags(cmd, &sf)
	cmd.Flags().StringVar(&state, "state", "", "convenience filter: state (open|closed|snoozed)")
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "convenience filter: updated_at > this Unix timestamp")
	return cmd
}

func (s *Service) newConversationGetCmd(token string) *cobra.Command {
	var id, displayAs string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one conversation (GET /conversations/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if displayAs != "" {
				q.Set("display_as", displayAs)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(id), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&displayAs, "display-as", "", "render conversation parts as plaintext when set to 'plaintext'")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newConversationReplyCmd posts a public comment (message_type=comment) — the
// customer-visible reply. Separate from `note` so answering the customer and
// leaving an internal note can never be confused for an agent.
func (s *Service) newConversationReplyCmd(token string) *cobra.Command {
	return s.newConversationReplyLike(token, "reply", "comment", "Reply to a conversation as the team (public comment)")
}

// newConversationNoteCmd posts an internal note (message_type=note) — visible
// only to admins.
func (s *Service) newConversationNoteCmd(token string) *cobra.Command {
	return s.newConversationReplyLike(token, "note", "note", "Add an internal note to a conversation (admins only)")
}

// newConversationReplyLike is the shared shape for reply/note: both POST
// /conversations/{id}/reply with type=admin and differ only by message_type.
func (s *Service) newConversationReplyLike(token, use, messageType, short string) *cobra.Command {
	var id, body, adminID string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"message_type": messageType,
				"type":         "admin",
				"admin_id":     admin,
				"body":         body,
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/"+url.PathEscape(id)+"/reply", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id (or 'last' for the latest part)")
	cmd.Flags().StringVar(&body, "body", "", "message body (HTML allowed)")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newConversationCloseCmd(token string) *cobra.Command {
	var id, body, adminID string
	cmd := &cobra.Command{
		Use:         "close",
		Short:       "Close a conversation (POST /conversations/{id}/parts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{"message_type": "close", "type": "admin", "admin_id": admin}
			if body != "" {
				payload["body"] = body
			}
			return s.postPart(cmd, token, id, payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&body, "body", "", "optional closing note")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newConversationOpenCmd(token string) *cobra.Command {
	var id, adminID string
	cmd := &cobra.Command{
		Use:         "open",
		Short:       "Reopen a snoozed or closed conversation (POST /conversations/{id}/parts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{"message_type": "open", "admin_id": admin}
			return s.postPart(cmd, token, id, payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newConversationSnoozeCmd(token string) *cobra.Command {
	var id, adminID, snoozedUntil string
	cmd := &cobra.Command{
		Use:         "snooze",
		Short:       "Snooze a conversation until a future time (POST /conversations/{id}/parts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{"message_type": "snoozed", "admin_id": admin, "snoozed_until": snoozedUntil}
			return s.postPart(cmd, token, id, payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&snoozedUntil, "snoozed-until", "", "Unix timestamp when the conversation should reopen")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("snoozed-until")
	return cmd
}

func (s *Service) newConversationAssignCmd(token string) *cobra.Command {
	var id, adminID, assigneeID, body string
	cmd := &cobra.Command{
		Use:         "assign",
		Short:       "Assign a conversation to an admin or team (POST /conversations/{id}/parts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"message_type": "assignment",
				"type":         "admin",
				"admin_id":     admin,
				"assignee_id":  assigneeID,
			}
			if body != "" {
				payload["body"] = body
			}
			return s.postPart(cmd, token, id, payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&assigneeID, "assignee-id", "", "target admin or team id (0 = unassigned)")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	cmd.Flags().StringVar(&body, "body", "", "optional assignment note")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("assignee-id")
	return cmd
}

// postPart POSTs a conversation-part payload and emits the response.
func (s *Service) postPart(cmd *cobra.Command, token, id string, payload map[string]any) error {
	resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/"+url.PathEscape(id)+"/parts", nil, payload)
	if err != nil {
		return err
	}
	return s.emit(resp)
}

func (s *Service) newConversationTagCmd(token string) *cobra.Command {
	var id, tagID, adminID string
	cmd := &cobra.Command{
		Use:         "tag",
		Short:       "Add a tag to a conversation (POST /conversations/{id}/tags)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{"id": tagID, "admin_id": admin}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/"+url.PathEscape(id)+"/tags", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&tagID, "tag-id", "", "tag id to add")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("tag-id")
	return cmd
}

func (s *Service) newConversationUntagCmd(token string) *cobra.Command {
	var id, tagID, adminID string
	cmd := &cobra.Command{
		Use:         "untag",
		Short:       "Remove a tag from a conversation (DELETE /conversations/{id}/tags/{tag_id})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{"admin_id": admin}
			resp, err := s.call(cmd.Context(), token, http.MethodDelete,
				"/conversations/"+url.PathEscape(id)+"/tags/"+url.PathEscape(tagID), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "conversation id")
	cmd.Flags().StringVar(&tagID, "tag-id", "", "tag id to remove")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("tag-id")
	return cmd
}
