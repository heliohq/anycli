package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// tasksPath is the tasks collection under a given list.
func tasksPath(list string) string {
	return "/lists/" + url.PathEscape(list) + "/tasks"
}

func (s *Service) newTasksListCmd(token string) *cobra.Command {
	var (
		max                                                  int
		pageToken                                            string
		dueAfter, dueBefore, completedAfter, completedBefore string
		updatedAfter                                         string
		showHidden, showDeleted, showAssigned                bool
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tasks in a list (tasks.list). No text search — the API has no query language; use --json and read.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, _ := cmd.Flags().GetString("list")
			q := url.Values{}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			setDue := func(key, v string) {
				if v != "" {
					q.Set(key, normalizeDue(v))
				}
			}
			setDue("dueMin", dueAfter)
			setDue("dueMax", dueBefore)
			setDue("completedMin", completedAfter)
			setDue("completedMax", completedBefore)
			setDue("updatedMin", updatedAfter)
			// showCompleted defaults to true API-side; only forward an explicit
			// override so the default stays the API's.
			if cmd.Flags().Changed("show-completed") {
				v, _ := cmd.Flags().GetBool("show-completed")
				q.Set("showCompleted", strconv.FormatBool(v))
			}
			if showHidden {
				q.Set("showHidden", "true")
			}
			if showDeleted {
				q.Set("showDeleted", "true")
			}
			if showAssigned {
				q.Set("showAssigned", "true")
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, tasksPath(list), q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Items         []task `json:"items"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("tasks: decode task list: %w", err)
			}
			if len(resp.Items) == 0 {
				fmt.Fprintln(s.stdout(), "no tasks")
				return nil
			}
			for _, t := range resp.Items {
				renderTaskRow(s.stdout(), t)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	listFlag(cmd)
	// tasks.list defaults to 20, caps at 100.
	addListPageFlags(cmd, &max, &pageToken, 20)
	cmd.Flags().StringVar(&dueAfter, "due-after", "", "only tasks due on/after this date (RFC3339 → dueMin)")
	cmd.Flags().StringVar(&dueBefore, "due-before", "", "only tasks due on/before this date (RFC3339 → dueMax)")
	cmd.Flags().StringVar(&completedAfter, "completed-after", "", "only tasks completed on/after this date (→ completedMin)")
	cmd.Flags().StringVar(&completedBefore, "completed-before", "", "only tasks completed on/before this date (→ completedMax)")
	cmd.Flags().StringVar(&updatedAfter, "updated-after", "", "only tasks updated on/after this date (→ updatedMin)")
	cmd.Flags().Bool("show-completed", true, "include completed tasks (API default true; --show-completed=false to hide)")
	cmd.Flags().BoolVar(&showHidden, "show-hidden", false, "include hidden tasks (completed via first-party clients)")
	cmd.Flags().BoolVar(&showDeleted, "show-deleted", false, "include deleted tasks (view only; no undelete path)")
	cmd.Flags().BoolVar(&showAssigned, "show-assigned", false, "include tasks assigned from Docs/Chat")
	return cmd
}

func (s *Service) newTasksGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <task-id>",
		Short:       "Show one task (tasks.get)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetString("list")
			body, err := s.call(cmd.Context(), token, http.MethodGet, tasksPath(list)+"/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var t task
			if err := json.Unmarshal(body, &t); err != nil {
				return fmt.Errorf("tasks: decode task: %w", err)
			}
			renderTaskDetail(s.stdout(), t)
			return nil
		},
	}
	listFlag(cmd)
	return cmd
}

func (s *Service) newTasksCreateCmd(token string) *cobra.Command {
	var title, notes, due, parent, previous string
	cmd := &cobra.Command{
		Use:         "create --title T",
		Short:       "Create a task (tasks.insert). --parent makes a subtask; --previous positions it.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, _ := cmd.Flags().GetString("list")
			payload := map[string]any{"title": title}
			if notes != "" {
				payload["notes"] = notes
			}
			if due != "" {
				payload["due"] = normalizeDue(due)
			}
			q := url.Values{}
			if parent != "" {
				q.Set("parent", parent)
			}
			if previous != "" {
				q.Set("previous", previous)
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, tasksPath(list), q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var t task
			if err := json.Unmarshal(body, &t); err != nil {
				return fmt.Errorf("tasks: decode task: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created task %s (%s)\n", t.Title, t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "task title (required)")
	cmd.Flags().StringVar(&notes, "notes", "", "task notes / body")
	cmd.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD or RFC3339; the API keeps only the date part)")
	cmd.Flags().StringVar(&parent, "parent", "", "parent task id (create as a subtask)")
	cmd.Flags().StringVar(&previous, "previous", "", "sibling task id to position after")
	listFlag(cmd)
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func (s *Service) newTasksUpdateCmd(token string) *cobra.Command {
	var title, notes, due string
	var clearDue bool
	cmd := &cobra.Command{
		Use:         "update <task-id>",
		Short:       "Update a task's fields (tasks.patch — partial update, never a full overwrite)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetString("list")
			payload := map[string]any{}
			if cmd.Flags().Changed("title") {
				payload["title"] = title
			}
			if cmd.Flags().Changed("notes") {
				payload["notes"] = notes
			}
			switch {
			case clearDue:
				payload["due"] = nil
			case due != "":
				payload["due"] = normalizeDue(due)
			}
			if len(payload) == 0 {
				return fmt.Errorf("tasks: nothing to update — pass --title, --notes, --due, or --clear-due")
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, tasksPath(list)+"/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var t task
			if err := json.Unmarshal(body, &t); err != nil {
				return fmt.Errorf("tasks: decode task: %w", err)
			}
			fmt.Fprintf(s.stdout(), "updated task %s (%s)\n", t.Title, t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&notes, "notes", "", "new notes")
	cmd.Flags().StringVar(&due, "due", "", "new due date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().BoolVar(&clearDue, "clear-due", false, "clear the due date")
	listFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("due", "clear-due")
	return cmd
}

// renderTaskRow prints a compact one-line task summary.
func renderTaskRow(w io.Writer, t task) {
	mark := "[ ]"
	if t.Status == statusCompleted {
		mark = "[x]"
	}
	line := fmt.Sprintf("%s %s\t%s", mark, t.ID, t.Title)
	if t.Due != "" {
		line += "\tdue=" + t.Due
	}
	if t.assigned() {
		line += "\t(assigned)"
	}
	fmt.Fprintln(w, line)
}

// renderTaskDetail prints the full human view of one task.
func renderTaskDetail(w io.Writer, t task) {
	fmt.Fprintf(w, "Id:     %s\nTitle:  %s\nStatus: %s\n", t.ID, t.Title, t.Status)
	if t.Due != "" {
		fmt.Fprintf(w, "Due:    %s\n", t.Due)
	}
	if t.Parent != "" {
		fmt.Fprintf(w, "Parent: %s\n", t.Parent)
	}
	if t.Notes != "" {
		fmt.Fprintf(w, "Notes:  %s\n", t.Notes)
	}
	if t.assigned() {
		fmt.Fprintln(w, "Assigned: yes (deleting cascades to the Docs/Chat original)")
	}
}
