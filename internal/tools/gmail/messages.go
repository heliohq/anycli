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
		Use:         "profile",
		Short:       "Show the connected mailbox profile (users.getProfile)",
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
		Use:         "list",
		Short:       "List messages (native Gmail search syntax via --query)",
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
		Use:         "get <message-id>",
		Short:       "Show one message: headers, body, and attachment inventory",
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
		Use:         "modify <message-id>...",
		Short:       "Add/remove labels (batchModify for multiple ids)",
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

// newMessagesTrashCmd builds trash (untrash=false) or untrash (untrash=true).
func (s *Service) newMessagesTrashCmd(token string, untrash bool) *cobra.Command {
	verb, past, short := "trash", "trashed", "Move messages to the trash"
	if untrash {
		verb, past, short = "untrash", "untrashed", "Move messages out of the trash"
	}
	return &cobra.Command{
		Use:         verb + " <message-id>...",
		Short:       short,
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
