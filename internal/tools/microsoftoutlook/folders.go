package microsoftoutlook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newFoldersListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List mail folders with unread/total counts (GET /me/mailFolders)",
		Long: "Returns folder ids alongside EXACT `unreadItemCount` and `totalItemCount`,\n" +
			"which is far cheaper than counting a `messages list` result and is the right\n" +
			"way to answer \"how many unread\". Fetches the top 100 folders in one call,\n" +
			"with no paging flag, and returns only top-level folders — nested child\n" +
			"folders are not expanded. The ids feed `messages list --folder` and\n" +
			"`messages move --folder`, both of which also accept well-known names.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("$top", "100")
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/mailFolders", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Value []struct {
					ID              string `json:"id"`
					DisplayName     string `json:"displayName"`
					UnreadItemCount int64  `json:"unreadItemCount"`
					TotalItemCount  int64  `json:"totalItemCount"`
				} `json:"value"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-outlook: decode folder list: %w", err)
			}
			if len(resp.Value) == 0 {
				fmt.Fprintln(s.stdout(), "no folders")
				return nil
			}
			for _, f := range resp.Value {
				fmt.Fprintf(s.stdout(), "%s\t%s\t(unread %d / total %d)\n", f.ID, f.DisplayName, f.UnreadItemCount, f.TotalItemCount)
			}
			return nil
		},
	}
	return cmd
}
