package microsoftoutlook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/msgraph"
	"github.com/spf13/cobra"
)

func (s *Service) newProfileCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Show the connected account profile (GET /me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var p struct {
				DisplayName       string `json:"displayName"`
				Mail              string `json:"mail"`
				UserPrincipalName string `json:"userPrincipalName"`
				ID                string `json:"id"`
			}
			if err := json.Unmarshal(body, &p); err != nil {
				return fmt.Errorf("microsoft-outlook: decode profile: %w", err)
			}
			mail := p.Mail
			if mail == "" {
				mail = p.UserPrincipalName
			}
			fmt.Fprintf(s.stdout(), "Name:  %s\nEmail: %s\nId:    %s\n", p.DisplayName, mail, p.ID)
			return nil
		},
	}
}

func (s *Service) newMessagesListCmd(token string) *cobra.Command {
	var search, filter, folder, page string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List messages (--search → Graph $search, --filter → OData $filter)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var body []byte
			var err error
			if page != "" {
				// @odata.nextLink is an absolute URL carrying the paging state.
				body, err = s.call(cmd.Context(), token, http.MethodGet, page, nil, nil)
			} else {
				q := url.Values{}
				if search != "" {
					q.Set("$search", `"`+search+`"`)
				}
				if filter != "" {
					q.Set("$filter", filter)
				}
				q.Set("$top", strconv.Itoa(max))
				path := "/me/messages"
				if folder != "" {
					path = "/me/mailFolders/" + url.PathEscape(folder) + "/messages"
				}
				body, err = s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			}
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Value    []graphMessage `json:"value"`
				NextLink string         `json:"@odata.nextLink"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-outlook: decode message list: %w", err)
			}
			if len(resp.Value) == 0 {
				fmt.Fprintln(s.stdout(), "no messages")
				return nil
			}
			for _, m := range resp.Value {
				from := ""
				if m.From != nil {
					from = m.From.EmailAddress.String()
				}
				unread := ""
				if !m.IsRead {
					unread = " [unread]"
				}
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\t%s%s\n", m.ID, m.ReceivedDateTime, from, m.Subject, unread)
			}
			if resp.NextLink != "" {
				fmt.Fprintf(s.stdout(), "next page: %s\n", resp.NextLink)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Graph $search query (passed through verbatim)")
	cmd.Flags().StringVar(&filter, "filter", "", "OData $filter expression (e.g. 'isRead eq false')")
	cmd.Flags().StringVar(&folder, "folder", "", "restrict to a folder id or well-known name (inbox, sentitems, ...)")
	cmd.Flags().IntVar(&max, "max", 10, "max results to return ($top)")
	cmd.Flags().StringVar(&page, "page", "", "@odata.nextLink from a previous list call")
	return cmd
}

func (s *Service) newMessagesGetCmd(token string) *cobra.Command {
	var bodyKind string
	var showHeaders bool
	cmd := &cobra.Command{
		Use:   "get <message-id>",
		Short: "Show one message: headers, body, and attachment inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodyKind != "text" && bodyKind != "html" {
				return fmt.Errorf("microsoft-outlook: --body must be text or html, got %q", bodyKind)
			}
			m, err := s.fetchMessage(cmd.Context(), token, args[0], bodyKind, showHeaders)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(m)
			}
			s.renderMessage(s.stdout(), m)
			return nil
		},
	}
	cmd.Flags().StringVar(&bodyKind, "body", "text", "body variant to show: text or html")
	cmd.Flags().BoolVar(&showHeaders, "headers", false, "show internet message headers")
	return cmd
}

// cleanMessageIDs splits every multi-id arg on whitespace and drops empties.
func cleanMessageIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("microsoft-outlook: no valid message ids")
	}
	return ids, nil
}

func (s *Service) newMessagesMoveCmd(token string) *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "move <message-id>...",
		Short: "Move messages to a folder ($batch for multiple ids)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanMessageIDs(args)
			if err != nil {
				return err
			}
			if folder == "" {
				return fmt.Errorf("microsoft-outlook: --folder is required")
			}
			payload := map[string]any{"destinationId": folder}
			if len(ids) == 1 {
				body, err := s.call(cmd.Context(), token, http.MethodPost, "/me/messages/"+url.PathEscape(ids[0])+"/move", nil, payload)
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				fmt.Fprintf(s.stdout(), "moved %s to %s\n", ids[0], folder)
				return nil
			}
			reqs := make([]batchSubRequest, 0, len(ids))
			for i, id := range ids {
				reqs = append(reqs, batchSubRequest{
					ID:      strconv.Itoa(i + 1),
					Method:  http.MethodPost,
					URL:     "/me/messages/" + url.PathEscape(id) + "/move",
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    payload,
				})
			}
			if err := s.batch(cmd.Context(), token, reqs); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "destinationId": folder, "status": "moved"})
			}
			fmt.Fprintf(s.stdout(), "moved %d message(s) to %s\n", len(ids), folder)
			return nil
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "destination folder id or well-known name")
	return cmd
}

func (s *Service) newMessagesMarkCmd(token string) *cobra.Command {
	var markRead, markUnread, flag, unflag bool
	cmd := &cobra.Command{
		Use:   "mark <message-id>...",
		Short: "Mark messages read/unread and/or flag/unflag ($batch for multiple ids)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanMessageIDs(args)
			if err != nil {
				return err
			}
			patch := map[string]any{}
			if markRead {
				patch["isRead"] = true
			}
			if markUnread {
				patch["isRead"] = false
			}
			if flag {
				patch["flag"] = map[string]any{"flagStatus": "flagged"}
			}
			if unflag {
				patch["flag"] = map[string]any{"flagStatus": "notFlagged"}
			}
			if len(patch) == 0 {
				return fmt.Errorf("microsoft-outlook: nothing to mark — pass --read, --unread, --flag, or --unflag")
			}
			if len(ids) == 1 {
				body, err := s.call(cmd.Context(), token, http.MethodPatch, "/me/messages/"+url.PathEscape(ids[0]), nil, patch)
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				fmt.Fprintf(s.stdout(), "marked %s\n", ids[0])
				return nil
			}
			reqs := make([]batchSubRequest, 0, len(ids))
			for i, id := range ids {
				reqs = append(reqs, batchSubRequest{
					ID:      strconv.Itoa(i + 1),
					Method:  http.MethodPatch,
					URL:     "/me/messages/" + url.PathEscape(id),
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    patch,
				})
			}
			if err := s.batch(cmd.Context(), token, reqs); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "status": "marked"})
			}
			fmt.Fprintf(s.stdout(), "marked %d message(s)\n", len(ids))
			return nil
		},
	}
	cmd.Flags().BoolVar(&markRead, "read", false, "mark as read")
	cmd.Flags().BoolVar(&markUnread, "unread", false, "mark as unread")
	cmd.Flags().BoolVar(&flag, "flag", false, "flag for follow-up")
	cmd.Flags().BoolVar(&unflag, "unflag", false, "clear the follow-up flag")
	cmd.MarkFlagsMutuallyExclusive("read", "unread")
	cmd.MarkFlagsMutuallyExclusive("flag", "unflag")
	return cmd
}

// batchSubRequest is one entry of a Graph JSON $batch request.
type batchSubRequest struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// batchMaxSize is Graph's per-$batch sub-request ceiling.
const batchMaxSize = 20

// batch posts sub-requests to /$batch in chunks of batchMaxSize and fails if
// any sub-response reports a non-2xx status.
func (s *Service) batch(ctx context.Context, token string, reqs []batchSubRequest) error {
	for start := 0; start < len(reqs); start += batchMaxSize {
		end := start + batchMaxSize
		if end > len(reqs) {
			end = len(reqs)
		}
		chunk := reqs[start:end]
		payload := map[string]any{"requests": chunk}
		body, err := s.call(ctx, token, http.MethodPost, "/$batch", nil, payload)
		if err != nil {
			return err
		}
		var resp struct {
			Responses []struct {
				ID     string          `json:"id"`
				Status int             `json:"status"`
				Body   json.RawMessage `json:"body"`
			} `json:"responses"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("microsoft-outlook: decode batch response: %w", err)
		}
		for _, r := range resp.Responses {
			if r.Status < 200 || r.Status > 299 {
				return fmt.Errorf("microsoft-outlook: batch request %s failed (HTTP %d): %s", r.ID, r.Status, msgraph.APIMessage(r.Body))
			}
		}
	}
	return nil
}
