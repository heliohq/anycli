package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newThreadsListCmd(token string) *cobra.Command {
	var query, pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List threads (native Gmail search syntax via --query)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if query != "" {
				q.Set("q", query)
			}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/threads", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Threads []struct {
					ID      string `json:"id"`
					Snippet string `json:"snippet"`
				} `json:"threads"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("gmail: decode thread list: %w", err)
			}
			if len(resp.Threads) == 0 {
				fmt.Fprintln(s.stdout(), "no threads")
				return nil
			}
			for _, t := range resp.Threads {
				fmt.Fprintf(s.stdout(), "%s\t%s\n", t.ID, truncate(t.Snippet, 80))
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Gmail search query (passed through verbatim)")
	addListFlags(cmd, &max, &pageToken)
	return cmd
}

func (s *Service) newThreadsGetCmd(token string) *cobra.Command {
	var bodyKind string
	cmd := &cobra.Command{
		Use:   "get <thread-id>",
		Short: "Show a whole conversation, messages in order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodyKind != "text" && bodyKind != "html" {
				return fmt.Errorf("gmail: --body must be text or html, got %q", bodyKind)
			}
			q := url.Values{"format": {"full"}}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/threads/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			var thread struct {
				ID       string    `json:"id"`
				Messages []*apiMsg `json:"messages"`
			}
			if err := json.Unmarshal(body, &thread); err != nil {
				return fmt.Errorf("gmail: decode thread: %w", err)
			}
			views := make([]messageView, 0, len(thread.Messages))
			for _, m := range thread.Messages {
				view, err := buildView(m, bodyKind, false)
				if err != nil {
					return err
				}
				views = append(views, view)
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"id": thread.ID, "messages": views})
			}
			for i, view := range views {
				fmt.Fprintf(s.stdout(), "--- message %d of %d ---\n", i+1, len(views))
				renderMessage(s.stdout(), view)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bodyKind, "body", "text", "body variant to show: text or html")
	return cmd
}

// truncate shortens a snippet for the human list view.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
