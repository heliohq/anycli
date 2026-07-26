package missive

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newConversationsCmd builds the `conversations` group: triage the shared
// inbox, read a thread (messages/comments/posts), and change conversation state.
func (s *Service) newConversationsCmd(token string) *cobra.Command {
	group := newGroupCmd("conversations", "Triage and read shared-inbox conversations")
	group.AddCommand(
		s.newConversationsListCmd(token),
		s.newConversationsGetCmd(token),
		s.newConversationsSubListCmd(token, "messages", "List messages on a conversation", longConversationsMessages, "delivered_at"),
		s.newConversationsSubListCmd(token, "comments", "List internal comments on a conversation", longConversationsComments, "created_at"),
		s.newConversationsSubListCmd(token, "posts", "List posts on a conversation", longConversationsPosts, "created_at"),
		s.newConversationsUpdateCmd(token),
	)
	return group
}

func (s *Service) newConversationsListCmd(token string) *cobra.Command {
	var (
		limit int
		until string

		inbox, all, assigned, closed  bool
		snoozed, flagged, trashed     bool
		junked, drafts                bool
		sharedLabel, teamInbox        string
		teamAll, teamClosed           string
		email, domain, contactOrgName string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List conversations in a mailbox (one mailbox filter required)",
		Long: "A MAILBOX filter is required: one of --inbox, --all, --assigned, --closed,\n" +
			"--snoozed, --flagged, --trashed, --junked, --drafts, or one of the\n" +
			"id-taking --shared-label, --team-inbox, --team-all, --team-closed. A bare\n" +
			"list does not default to the inbox — Missive rejects it with a 400. The\n" +
			"narrowing filters --email, --domain and --contact-organization are\n" +
			"mutually exclusive and the combination is refused before any request goes\n" +
			"out. --limit defaults to 25 and Missive caps it at 50; continue with\n" +
			"--until from `next_until`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// email / domain / contact_organization are mutually exclusive; the
			// API 400s if combined, so reject locally to save a round trip.
			if err := ensureAtMostOneNarrowing(email, domain, contactOrgName); err != nil {
				return err
			}
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if until != "" {
				q.Set("until", until)
			}
			setBoolFilter(q, "inbox", inbox)
			setBoolFilter(q, "all", all)
			setBoolFilter(q, "assigned", assigned)
			setBoolFilter(q, "closed", closed)
			setBoolFilter(q, "snoozed", snoozed)
			setBoolFilter(q, "flagged", flagged)
			setBoolFilter(q, "trashed", trashed)
			setBoolFilter(q, "junked", junked)
			setBoolFilter(q, "drafts", drafts)
			setStr(q, "shared_label", sharedLabel)
			setStr(q, "team_inbox", teamInbox)
			setStr(q, "team_all", teamAll)
			setStr(q, "team_closed", teamClosed)
			setStr(q, "email", email)
			setStr(q, "domain", domain)
			setStr(q, "contact_organization", contactOrgName)

			// Missive requires at least one mailbox filter; when absent the API
			// returns a 400 that is surfaced verbatim rather than pre-validated.
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations", q, nil)
			if err != nil {
				return err
			}
			return s.emitUntilList(body, "conversations", "last_activity_at")
		},
	}
	f := cmd.Flags()
	f.IntVar(&limit, "limit", 25, "max conversations (Missive max 50)")
	f.StringVar(&until, "until", "", "pagination cursor: last_activity_at of the oldest conversation from the previous page")
	f.BoolVar(&inbox, "inbox", false, "mailbox filter: inbox")
	f.BoolVar(&all, "all", false, "mailbox filter: all")
	f.BoolVar(&assigned, "assigned", false, "mailbox filter: assigned")
	f.BoolVar(&closed, "closed", false, "mailbox filter: closed")
	f.BoolVar(&snoozed, "snoozed", false, "mailbox filter: snoozed")
	f.BoolVar(&flagged, "flagged", false, "mailbox filter: flagged")
	f.BoolVar(&trashed, "trashed", false, "mailbox filter: trashed")
	f.BoolVar(&junked, "junked", false, "mailbox filter: junked")
	f.BoolVar(&drafts, "drafts", false, "mailbox filter: drafts")
	f.StringVar(&sharedLabel, "shared-label", "", "mailbox filter: shared label id")
	f.StringVar(&teamInbox, "team-inbox", "", "mailbox filter: team inbox id")
	f.StringVar(&teamAll, "team-all", "", "mailbox filter: team all id")
	f.StringVar(&teamClosed, "team-closed", "", "mailbox filter: team closed id")
	f.StringVar(&email, "email", "", "narrow to conversations with this email (mutually exclusive with --domain/--contact-organization)")
	f.StringVar(&domain, "domain", "", "narrow to conversations with this domain")
	f.StringVar(&contactOrgName, "contact-organization", "", "narrow to conversations with this contact organization")
	return cmd
}

func (s *Service) newConversationsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <conversation-id>",
		Short: "Show one conversation",
		Long: "Returns the conversation's own state — subject, assignees, shared labels,\n" +
			"open or closed, last activity — and none of its contents. The thread is\n" +
			"three separate sub-lists: `conversations messages` for customer traffic,\n" +
			"`conversations comments` for human internal notes, `conversations posts`\n" +
			"for integration-written ones.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// longConversationsMessages, longConversationsComments and
// longConversationsPosts sit beside the shared sub-list builder because it is
// the builder that fixes the 10-item ceiling and the cursor field all three
// describe.
const (
	longConversationsMessages = "The customer-facing traffic on this conversation — email, SMS and chat,\n" +
		"inbound and outbound alike. Internal team notes are NOT here; they are\n" +
		"`conversations comments` and `conversations posts`. --limit defaults to 10\n" +
		"and 10 is also Missive's maximum, so a long thread takes several pages\n" +
		"through --until, whose cursor here is a delivery timestamp."

	longConversationsComments = "Internal team notes typed by people in Missive; the customer never sees\n" +
		"them. The cursor is a creation timestamp rather than the delivery timestamp\n" +
		"`conversations messages` returns, so an --until value cannot be carried\n" +
		"between the two lists. --limit defaults to 10, which is Missive's maximum."

	longConversationsPosts = "Posts are what integrations write into a conversation, including everything\n" +
		"`posts create` produced — distinct from the human-typed notes under\n" +
		"`conversations comments`, which is why an automated summary does not appear\n" +
		"there. --limit defaults to 10, Missive's maximum, paged with --until on a\n" +
		"creation timestamp."
)

// newConversationsSubListCmd builds a cursor-paged thread sub-resource lister
// (messages / comments / posts). subPath is both the URL segment and the
// response's top-level key; cursorField is the until cursor for that resource.
func (s *Service) newConversationsSubListCmd(token, subPath, short, long, cursorField string) *cobra.Command {
	var (
		limit int
		until string
	)
	cmd := &cobra.Command{
		Use:         subPath + " <conversation-id>",
		Short:       short,
		Long:        long,
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if until != "" {
				q.Set("until", until)
			}
			path := "/conversations/" + url.PathEscape(args[0]) + "/" + subPath
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emitUntilList(body, subPath, cursorField)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "max items (Missive max 10)")
	cmd.Flags().StringVar(&until, "until", "", "pagination cursor from the previous page")
	return cmd
}

func (s *Service) newConversationsUpdateCmd(token string) *cobra.Command {
	var inline, file string
	cmd := &cobra.Command{
		Use:   "update <conversation-id>",
		Short: "Change conversation state (close/assign/label) via PATCH. Body: {\"conversations\":{...}}",
		Long: "The body wraps under `conversations` and its field names are verbs:\n" +
			"`close`, `reopen`, `add_shared_labels`, `remove_shared_labels`,\n" +
			"`add_assignees`, `remove_assignees`. Labels and assignees are added and\n" +
			"removed as deltas rather than replaced wholesale, so the current state\n" +
			"does not have to be read first. Everything this command changes is\n" +
			"internal team state — nothing is sent to the customer.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := s.decodeJSONBody(inline, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/conversations/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emitBodyOrOK(body)
		},
	}
	addBodyFlags(cmd, &inline, &file)
	return cmd
}

// setStr sets key=value on q only when value is non-empty.
func setStr(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// ensureAtMostOneNarrowing rejects combining Missive's mutually-exclusive
// conversation narrowing filters (email/domain/contact_organization).
func ensureAtMostOneNarrowing(email, domain, org string) error {
	n := 0
	for _, v := range []string{email, domain, org} {
		if v != "" {
			n++
		}
	}
	if n > 1 {
		return &usageError{msg: "--email, --domain, and --contact-organization are mutually exclusive; pass at most one"}
	}
	return nil
}
