package bluesky

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newNotificationsCmd(sess *session) *cobra.Command {
	cmd := &cobra.Command{Use: "notifications", Short: "Notifications"}
	var limit int
	var cursor string
	list := &cobra.Command{
		Use:   "list",
		Short: "List notifications (mentions, replies, likes, follows) — one page",
		Long: "Each entry carries a `reason` naming which kind it is, the author, the\n" +
			"at:// uri and cid of the subject, and `is_read`. Reading here does not\n" +
			"mark anything read — no command moves the read cursor — so `is_read`\n" +
			"only reflects what other Bluesky clients have done, and a repeat call\n" +
			"returns the same entries. --limit is 1-100, default 25; continue with\n" +
			"--cursor.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := feedQuery(limit, cursor)
			if err != nil {
				return err
			}
			body, err := sess.call(cmd.Context(), http.MethodGet, "app.bsky.notification.listNotifications", query, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Notifications []struct {
					URI    string `json:"uri"`
					CID    string `json:"cid"`
					Reason string `json:"reason"`
					Author struct {
						DID         string `json:"did"`
						Handle      string `json:"handle"`
						DisplayName string `json:"displayName"`
					} `json:"author"`
					IsRead    bool   `json:"isRead"`
					IndexedAt string `json:"indexedAt"`
				} `json:"notifications"`
				Cursor string `json:"cursor"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("bluesky: decode notifications: %w", err)
			}
			items := make([]map[string]any, 0, len(resp.Notifications))
			for _, n := range resp.Notifications {
				items = append(items, map[string]any{
					"uri":        n.URI,
					"cid":        n.CID,
					"reason":     n.Reason,
					"author":     authorView{DID: n.Author.DID, Handle: n.Author.Handle, DisplayName: n.Author.DisplayName},
					"is_read":    n.IsRead,
					"indexed_at": n.IndexedAt,
				})
			}
			return s.emitValue(map[string]any{"notifications": items, "cursor": resp.Cursor})
		},
	}
	addFeedFlags(list, &limit, &cursor)
	cmd.AddCommand(list)
	return cmd
}
