package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newProfileCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Show the connected mailbox profile (users.getProfile)",
		Long: "Reports the connected mailbox's own address plus `messagesTotal`,\n" +
			"`threadsTotal` and the current `historyId`. Those totals span the whole\n" +
			"account including spam and trash, so they are NOT an inbox figure —\n" +
			"`labels get INBOX` is what answers that. `messages reply --all` calls this\n" +
			"internally to keep the mailbox off its own Cc line.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/profile", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var p struct {
				EmailAddress  string `json:"emailAddress"`
				MessagesTotal int64  `json:"messagesTotal"`
				ThreadsTotal  int64  `json:"threadsTotal"`
				HistoryID     string `json:"historyId"`
			}
			if err := json.Unmarshal(body, &p); err != nil {
				return fmt.Errorf("gmail: decode profile: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Email:    %s\nMessages: %d\nThreads:  %d\nHistory:  %s\n",
				p.EmailAddress, p.MessagesTotal, p.ThreadsTotal, p.HistoryID)
			return nil
		},
	}
}

// addListFlags wires the shared list pagination flags.
func addListFlags(cmd *cobra.Command, max *int, pageToken *string) {
	cmd.Flags().IntVar(max, "max", 10, "max results to return")
	cmd.Flags().StringVar(pageToken, "page-token", "", "page token from a previous list call")
}

func (s *Service) newMessagesListCmd(token string) *cobra.Command {
	var query, pageToken string
	var labels []string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List messages (native Gmail search syntax via --query)",
		Long: "Returns ids only — no sender, subject or snippet — so triage is this plus a\n" +
			"`messages get` per interesting id, or `threads list` when a snippet is\n" +
			"enough. `--label` restricts to a label id from `labels list`, never a label\n" +
			"name. `--max` defaults to 10 and Gmail caps it at 500; the response's\n" +
			"`nextPageToken` goes back in as `--page-token`.\n" +
			"\n" +
			"Under `--json` the reply carries `resultSizeEstimate`. It is Gmail's cheap\n" +
			"index estimate, it saturates around 100-200 regardless of the real total,\n" +
			"and it must NEVER be reported as a count — only zero versus non-zero is\n" +
			"meaningful. For an exact figure use `labels get <label-id>`.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if query != "" {
				q.Set("q", query)
			}
			for _, label := range labels {
				q.Add("labelIds", label)
			}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/messages", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Messages []struct {
					ID       string `json:"id"`
					ThreadID string `json:"threadId"`
				} `json:"messages"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("gmail: decode message list: %w", err)
			}
			if len(resp.Messages) == 0 {
				fmt.Fprintln(s.stdout(), "no messages")
				return nil
			}
			for _, m := range resp.Messages {
				fmt.Fprintf(s.stdout(), "%s\tthread=%s\n", m.ID, m.ThreadID)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Gmail search query (passed through verbatim)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "restrict to a label id (repeatable)")
	addListFlags(cmd, &max, &pageToken)
	return cmd
}

func (s *Service) newMessagesGetCmd(token string) *cobra.Command {
	var bodyKind string
	var showHeaders bool
	cmd := &cobra.Command{
		Use:   "get <message-id>",
		Short: "Show one message: headers, body, and attachment inventory",
		Long: "`--body` selects the `text` or `html` variant and defaults to text, which\n" +
			"matters for mail that carries no plain-text part. `--headers` adds the full\n" +
			"header set and is off by default. The attachments are numbered in the order\n" +
			"`messages attachments --index` counts them, but their `attachmentId`s are\n" +
			"regenerated on every fetch and cannot be carried to a later call.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodyKind != "text" && bodyKind != "html" {
				return fmt.Errorf("gmail: --body must be text or html, got %q", bodyKind)
			}
			m, err := s.fetchMessage(cmd.Context(), token, args[0])
			if err != nil {
				return err
			}
			view, err := buildView(m, bodyKind, showHeaders)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(view)
			}
			renderMessage(s.stdout(), view)
			return nil
		},
	}
	cmd.Flags().StringVar(&bodyKind, "body", "text", "body variant to show: text or html")
	cmd.Flags().BoolVar(&showHeaders, "headers", false, "show all message headers")
	return cmd
}

// cleanMessageIDs splits every multi-id arg on whitespace and drops empties.
// Gmail returns INVALID_ARGUMENT for ids carrying ANY whitespace (trailing
// spaces, \r from pipelines, several ids pasted into one arg); message ids
// never contain whitespace, so Fields-splitting is always safe and kills the
// whole invisible-whitespace class rather than only leading/trailing runs.
func cleanMessageIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("gmail: no valid message ids")
	}
	return ids, nil
}

func (s *Service) newMessagesModifyCmd(token string) *cobra.Command {
	var addLabels, removeLabels []string
	var archive, markRead, markUnread bool
	cmd := &cobra.Command{
		Use:   "modify <message-id>...",
		Short: "Add/remove labels (batchModify for multiple ids)",
		Long: "Takes label IDS, not names: the system ids `INBOX`, `UNREAD`, `STARRED` and\n" +
			"friends, or a `Label_…` id from `labels list`. `--archive` is exactly\n" +
			"`--remove-label INBOX` and `--mark-read` exactly `--remove-label UNREAD`,\n" +
			"and at least one of the five flags must be given or the command refuses.\n" +
			"\n" +
			"One id goes through messages.modify and returns the updated message; two or\n" +
			"more go through batchModify, which returns NO bodies, so the receipt there\n" +
			"is built from the request rather than from Gmail. Label changes are\n" +
			"reversible by applying the opposite ones. Only the ids passed are touched —\n" +
			"a query never reaches this command, so scale is exactly what was listed.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanMessageIDs(args)
			if err != nil {
				return err
			}
			add := append([]string{}, addLabels...)
			remove := append([]string{}, removeLabels...)
			if archive {
				remove = append(remove, "INBOX")
			}
			if markRead {
				remove = append(remove, "UNREAD")
			}
			if markUnread {
				add = append(add, "UNREAD")
			}
			if len(add) == 0 && len(remove) == 0 {
				return fmt.Errorf("gmail: nothing to modify — pass --add-label, --remove-label, --archive, --mark-read, or --mark-unread")
			}
			if len(ids) == 1 {
				payload := map[string]any{"addLabelIds": add, "removeLabelIds": remove}
				body, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/messages/"+url.PathEscape(ids[0])+"/modify", nil, payload)
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				fmt.Fprintf(s.stdout(), "modified %s\n", ids[0])
				return nil
			}
			payload := map[string]any{"ids": ids, "addLabelIds": add, "removeLabelIds": remove}
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/messages/batchModify", nil, payload); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "addLabelIds": add, "removeLabelIds": remove, "status": "modified"})
			}
			fmt.Fprintf(s.stdout(), "modified %d messages\n", len(ids))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&addLabels, "add-label", nil, "label id to add (repeatable)")
	cmd.Flags().StringArrayVar(&removeLabels, "remove-label", nil, "label id to remove (repeatable)")
	cmd.Flags().BoolVar(&archive, "archive", false, "archive (remove INBOX)")
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark as read (remove UNREAD)")
	cmd.Flags().BoolVar(&markUnread, "mark-unread", false, "mark as unread (add UNREAD)")
	cmd.MarkFlagsMutuallyExclusive("mark-read", "mark-unread")
	return cmd
}

// longMessagesTrash and longMessagesUntrash are the two Longs for the pair of
// commands newMessagesTrashCmd builds. They live next to the shared builder
// because it is the builder that fixes the direction each one describes.
const (
	longMessagesTrash = "Reversible: `messages untrash` puts a message back with the labels it had,\n" +
		"and Gmail keeps trashed mail for 30 days before purging it for good — after\n" +
		"which nothing here recovers it. The ids are trashed one request at a time\n" +
		"and the run stops at the first failure, leaving the earlier ones already\n" +
		"trashed. It acts only on the ids given, never on a query, so nothing is\n" +
		"trashed that was not listed first."

	longMessagesUntrash = "The inverse of `messages trash`, restoring each message to the labels it\n" +
		"carried before. It only reaches mail still IN the trash: Gmail purges\n" +
		"trashed messages after 30 days and there is no command here that recovers\n" +
		"one afterwards. Like trashing, the ids are applied one request at a time and\n" +
		"the run stops at the first failure."
)

// newMessagesTrashCmd builds trash (untrash=false) or untrash (untrash=true).
func (s *Service) newMessagesTrashCmd(token string, untrash bool) *cobra.Command {
	verb, past, short, long := "trash", "trashed", "Move messages to the trash", longMessagesTrash
	if untrash {
		verb, past, short, long = "untrash", "untrashed", "Move messages out of the trash", longMessagesUntrash
	}
	return &cobra.Command{
		Use:         verb + " <message-id>...",
		Short:       short,
		Long:        long,
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanMessageIDs(args)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/messages/"+url.PathEscape(id)+"/"+verb, nil, nil); err != nil {
					return err
				}
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "status": past})
			}
			fmt.Fprintf(s.stdout(), "%s %d message(s)\n", past, len(ids))
			return nil
		},
	}
}
