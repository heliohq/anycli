package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// tasklistsPath is the tasklists resource collection under the API base.
const tasklistsPath = "/users/@me/lists"

func (s *Service) newListsListCmd(token string) *cobra.Command {
	var max int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List task lists (tasklists.list)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, tasklistsPath, q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Items         []taskList `json:"items"`
				NextPageToken string     `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("tasks: decode task list index: %w", err)
			}
			if len(resp.Items) == 0 {
				fmt.Fprintln(s.stdout(), "no task lists")
				return nil
			}
			for _, l := range resp.Items {
				fmt.Fprintf(s.stdout(), "%s\t%s\n", l.ID, l.Title)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	// tasklists.list allows maxResults up to 1000 (its API default and cap);
	// the tool intentionally caps its own default at 100 to keep a bare
	// `lists list` cheap, and forwards higher --max values unchanged.
	addListPageFlags(cmd, &max, &pageToken, 100)
	return cmd
}

func (s *Service) newListsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <list-id>",
		Short: "Show one task list (tasklists.get)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, tasklistsPath+"/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var l taskList
			if err := json.Unmarshal(body, &l); err != nil {
				return fmt.Errorf("tasks: decode task list: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Id:    %s\nTitle: %s\n", l.ID, l.Title)
			return nil
		},
	}
}

func (s *Service) newListsCreateCmd(token string) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "create --title T",
		Short: "Create a task list (tasklists.insert)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, tasklistsPath, nil, map[string]string{"title": title})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var l taskList
			if err := json.Unmarshal(body, &l); err != nil {
				return fmt.Errorf("tasks: decode task list: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created task list %s (%s)\n", l.Title, l.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "task list title (required)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func (s *Service) newListsUpdateCmd(token string) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "update <list-id> --title T",
		Short: "Rename a task list (tasklists.patch; title is the only writable field)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPatch, tasklistsPath+"/"+url.PathEscape(args[0]), nil, map[string]string{"title": title})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var l taskList
			if err := json.Unmarshal(body, &l); err != nil {
				return fmt.Errorf("tasks: decode task list: %w", err)
			}
			fmt.Fprintf(s.stdout(), "updated task list %s (%s)\n", l.Title, l.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new task list title (required)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func (s *Service) newListsDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <list-id>",
		Short: "Delete a task list and every task in it (tasklists.delete) — irreversible; assigned-task originals in Docs/Chat are deleted too",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, tasklistsPath+"/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"id": args[0], "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted task list %s\n", args[0])
			return nil
		},
	}
}
