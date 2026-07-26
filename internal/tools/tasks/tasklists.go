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
		Long: "Every task list on the account, each with the id that `--list` takes\n" +
			"everywhere else and the title the user sees. The primary list also answers\n" +
			"to the alias `@default`, which is what the task verbs use when `--list` is\n" +
			"omitted. Titles are not unique, so two lists can share a name and only the\n" +
			"id tells them apart.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
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
		Long: "Returns a single list's title and last-updated time. It does NOT include\n" +
			"the list's tasks or even a count of them — `list --list <list-id>` is the\n" +
			"only way to see what a list holds, and therefore the only way to know what\n" +
			"a `lists delete` would take with it.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
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
		Long: "Creates an empty list; `--title` is the only field a task list has. Titles\n" +
			"are not checked for uniqueness, so calling this twice yields two lists with\n" +
			"the same name and different ids. The returned id is what `--list` takes on\n" +
			"every task verb.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
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
		Long: "A rename and nothing else: `title` is the only writable field on a task\n" +
			"list, so ordering, colour and sharing cannot be changed through the API.\n" +
			"The list id is unaffected, so existing `--list` references keep working.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
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
		Long: "Destroys the list AND every task inside it, irreversibly — including the\n" +
			"Docs or Chat originals behind any assigned tasks it holds. The API gives no\n" +
			"warning and no count, so `list --list <list-id>` is the only way to see\n" +
			"what would be lost. Google refuses to delete the account's primary list.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
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
