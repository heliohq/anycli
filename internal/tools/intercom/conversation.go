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
		Use:   "list",
		Short: "List conversations (GET /conversations)",
		Long: "An unfiltered walk of the whole inbox, oldest state and all,\n" +
			"cursor-paginated at Intercom's default of 20 per page and a ceiling of\n" +
			"150. It cannot filter, so answering anything shaped like \"which\n" +
			"conversations are still open\" means `conversation search --state open`\n" +
			"instead of paging this.",
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
		Use:   "search",
		Short: "Search conversations (POST /conversations/search)",
		Long: "The convenience filters are --state (open, closed or snoozed) and\n" +
			"--updated-since, which compiles to `updated_at >` a Unix timestamp in\n" +
			"seconds; passing both ANDs them. Anything else — assignee, tag, contact,\n" +
			"priority — needs a raw --query object built from Intercom's\n" +
			"field/operator/value grammar, and --query cannot be combined with the\n" +
			"convenience flags. A call with neither is rejected rather than returning\n" +
			"everything.",
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
		Use:   "get",
		Short: "Get one conversation (GET /conversations/{id})",
		Long: "Returns the conversation with its parts — the full message thread, not\n" +
			"just the header the list and search results carry. Part bodies are HTML by\n" +
			"default; --display-as plaintext strips the markup, which is both cheaper\n" +
			"to read and less likely to be mis-parsed.",
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

// longConversationReply and longConversationNote are the public/internal pair.
// They sit next to the shared builder because it is the builder that fixes the
// single endpoint and the message_type both describe.
const (
	longConversationReply = "Posts a public comment attributed to the acting admin. The customer sees\n" +
		"it as soon as the call returns: there is no draft, no edit and no command\n" +
		"to remove a conversation part afterwards. --body accepts HTML. For a\n" +
		"teammate-only remark on the same conversation use `conversation note`,\n" +
		"which hits the same endpoint and differs only in message type."

	longConversationNote = "Adds a note visible only to teammates; the customer never sees it and it\n" +
		"does not count as a response. It shares the endpoint and the admin\n" +
		"attribution of `conversation reply` and differs only in message type, so\n" +
		"reaching for the wrong verb publishes a private aside to the customer.\n" +
		"Context about the PERSON rather than this thread belongs in\n" +
		"`contact note`, which survives across conversations."
)

// newConversationReplyCmd posts a public comment (message_type=comment) — the
// customer-visible reply. Separate from `note` so answering the customer and
// leaving an internal note can never be confused for an agent.
func (s *Service) newConversationReplyCmd(token string) *cobra.Command {
	return s.newConversationReplyLike(token, "reply", "comment",
		"Reply to a conversation as the team (public comment)", longConversationReply)
}

// newConversationNoteCmd posts an internal note (message_type=note) — visible
// only to admins.
func (s *Service) newConversationNoteCmd(token string) *cobra.Command {
	return s.newConversationReplyLike(token, "note", "note",
		"Add an internal note to a conversation (admins only)", longConversationNote)
}

// newConversationReplyLike is the shared shape for reply/note: both POST
// /conversations/{id}/reply with type=admin and differ only by message_type.
func (s *Service) newConversationReplyLike(token, use, messageType, short, long string) *cobra.Command {
	var id, body, adminID string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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
		Use:   "close",
		Short: "Close a conversation (POST /conversations/{id}/parts)",
		Long: "Closing is not destructive: the conversation and every part stay readable,\n" +
			"and `conversation open` puts it back. Intercom also reopens a closed\n" +
			"conversation by itself when the customer writes again. --body is optional\n" +
			"and is recorded on the closing part rather than kept private, so an\n" +
			"internal remark belongs in `conversation note` first.",
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
		Use:   "open",
		Short: "Reopen a snoozed or closed conversation (POST /conversations/{id}/parts)",
		Long: "Works from either closed or snoozed, and on a snoozed conversation it\n" +
			"cancels the pending wake-up rather than merely marking it open.\n" +
			"Already-open conversations are unaffected, so this is safe to issue\n" +
			"without checking state first.",
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
		Use:   "snooze",
		Short: "Snooze a conversation until a future time (POST /conversations/{id}/parts)",
		Long: "--snoozed-until is a required Unix timestamp in SECONDS, not milliseconds,\n" +
			"and must be in the future. The conversation reopens by itself at that\n" +
			"moment, and earlier than that if the customer replies in the meantime.\n" +
			"`conversation open` cancels a snooze that is still pending.",
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
		Use:   "assign",
		Short: "Assign a conversation to an admin or team (POST /conversations/{id}/parts)",
		Long: "--assignee-id is who receives the conversation and --admin-id is who\n" +
			"performs the assignment; they are different roles, not alternatives. The\n" +
			"assignee may be an admin id from `admin list` or a team id from `team\n" +
			"list` — one flag, two id spaces, so an id from the wrong list fails rather\n" +
			"than resolving to something unexpected. `0` unassigns. An optional --body\n" +
			"rides along on the assignment part.",
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
		Use:   "tag",
		Short: "Add a tag to a conversation (POST /conversations/{id}/tags)",
		Long: "--tag-id is an id from `tag list`, never a tag name; a tag that does not\n" +
			"exist yet has to be created with `tag create` first. The tagging is\n" +
			"attributed to an admin, so omitting --admin-id costs an extra /me lookup.",
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
		Use:   "untag",
		Short: "Remove a tag from a conversation (DELETE /conversations/{id}/tags/{tag_id})",
		Long: "Detaches the tag from this conversation only; the tag itself survives in\n" +
			"the workspace. This is the only untag verb in the tool — a tag applied\n" +
			"with `contact tag` cannot be removed from here.",
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
