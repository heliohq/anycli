package tasks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// Task status values. complete / reopen are synthetic verbs over tasks.patch —
// the highest-frequency actions in this product earn first-class commands.
const (
	statusCompleted   = "completed"
	statusNeedsAction = "needsAction"
)

// newTasksStatusCmd builds `complete` (status=completed) or `reopen`
// (status=needsAction). Both patch the status field; multiple ids are patched
// serially and failures are reported per id (the API has no batch-modify verb).
func (s *Service) newTasksStatusCmd(token, status string) *cobra.Command {
	verb, past, short := "complete", "completed", "Mark tasks completed (synthetic verb = patch status=completed)"
	if status == statusNeedsAction {
		verb, past, short = "reopen", "reopened", "Reopen completed tasks (synthetic verb = patch status=needsAction)"
	}
	cmd := &cobra.Command{
		Use:         verb + " <task-id>...",
		Short:       short,
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetString("list")
			ids, err := splitIDs(args)
			if err != nil {
				return err
			}
			return s.applyPerID(cmd, ids, past, func(ctx context.Context, id string) error {
				_, err := s.call(ctx, token, http.MethodPatch, tasksPath(list)+"/"+url.PathEscape(id), nil, map[string]any{"status": status})
				return err
			})
		},
	}
	listFlag(cmd)
	return cmd
}

func (s *Service) newTasksMoveCmd(token string) *cobra.Command {
	var parent, previous, toList string
	cmd := &cobra.Command{
		Use:         "move <task-id>",
		Short:       "Reposition/reparent a task (tasks.move). --to-list moves it to another list (repeating tasks cannot cross lists — API 400 passes through).",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetString("list")
			q := url.Values{}
			if parent != "" {
				q.Set("parent", parent)
			}
			if previous != "" {
				q.Set("previous", previous)
			}
			if toList != "" {
				q.Set("destinationTasklist", toList)
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, tasksPath(list)+"/"+url.PathEscape(args[0])+"/move", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "moved task %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "new parent task id")
	cmd.Flags().StringVar(&previous, "previous", "", "sibling task id to position after")
	cmd.Flags().StringVar(&toList, "to-list", "", "destination task list id (destinationTasklist)")
	listFlag(cmd)
	return cmd
}

func (s *Service) newTasksClearCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "clear",
		Short:       "Hide all completed tasks in a list (tasks.clear). Reversible — --show-hidden re-reveals them; prefer this over delete for 'clean up done'.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, _ := cmd.Flags().GetString("list")
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/lists/"+url.PathEscape(list)+"/clear", nil, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"list": list, "status": "cleared"})
			}
			fmt.Fprintf(s.stdout(), "cleared completed tasks in %s\n", list)
			return nil
		},
	}
	listFlag(cmd)
	return cmd
}

func (s *Service) newTasksDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <task-id>...",
		Short:       "Delete tasks (tasks.delete) — irreversible; an assigned task's Docs/Chat original is deleted too. Prefer `clear` for done tasks.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetString("list")
			ids, err := splitIDs(args)
			if err != nil {
				return err
			}
			return s.applyPerID(cmd, ids, "deleted", func(ctx context.Context, id string) error {
				_, err := s.call(ctx, token, http.MethodDelete, tasksPath(list)+"/"+url.PathEscape(id), nil, nil)
				return err
			})
		},
	}
	listFlag(cmd)
	return cmd
}

// perIDOutcome is one result row for a serial multi-id operation.
type perIDOutcome struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// applyPerID runs op against each id serially, reporting per-id outcomes and
// continuing past failures (design 303: "failed ids reported one by one"). It
// returns a non-nil error when any id failed so the exit code is non-zero, and
// preserves credential rejection when the provider rejected the token.
func (s *Service) applyPerID(cmd *cobra.Command, ids []string, past string, op func(context.Context, string) error) error {
	results := make([]perIDOutcome, 0, len(ids))
	var firstErr error
	credRejected, failed := false, 0
	for _, id := range ids {
		if err := op(cmd.Context(), id); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			if execution.IsCredentialRejected(err) {
				credRejected = true
			}
			results = append(results, perIDOutcome{ID: id, Status: "error", Error: err.Error()})
			continue
		}
		results = append(results, perIDOutcome{ID: id, Status: past})
	}

	if jsonOut(cmd) {
		if err := s.emitJSON(map[string]any{"results": results}); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			if r.Error != "" {
				// For a single id the returned error is printed by Execute;
				// only echo per-id failure lines when there are several.
				if len(ids) > 1 {
					fmt.Fprintf(s.stderr(), "failed %s: %s\n", r.ID, r.Error)
				}
				continue
			}
			fmt.Fprintf(s.stdout(), "%s %s\n", r.Status, r.ID)
		}
	}

	if failed == 0 {
		return nil
	}
	if len(ids) == 1 {
		return firstErr
	}
	summary := fmt.Errorf("tasks: %d of %d operations failed", failed, len(ids))
	if credRejected {
		return execution.RejectCredential(summary)
	}
	return summary
}
