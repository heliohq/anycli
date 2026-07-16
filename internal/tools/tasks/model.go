package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// task is the decode shape for a Tasks API task resource (only the fields the
// human summary renders). --json always emits the raw provider body.
type task struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Due            string          `json:"due"`
	Notes          string          `json:"notes"`
	Parent         string          `json:"parent"`
	Deleted        bool            `json:"deleted"`
	Hidden         bool            `json:"hidden"`
	AssignmentInfo *map[string]any `json:"assignmentInfo,omitempty"`
}

// taskList is the decode shape for a tasklists resource.
type taskList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// listFlag registers the shared `--list` selector (defaults to the primary
// list alias @default) and returns a pointer to its value.
func listFlag(cmd *cobra.Command) *string {
	list := new(string)
	cmd.Flags().StringVar(list, "list", defaultList, "task list id (defaults to the primary list @default)")
	return list
}

// addListPageFlags wires the shared list pagination flags with a resource
// specific default page size.
func addListPageFlags(cmd *cobra.Command, max *int, pageToken *string, defaultMax int) {
	cmd.Flags().IntVar(max, "max", defaultMax, "max results per page")
	cmd.Flags().StringVar(pageToken, "page-token", "", "page token from a previous list call")
}

// normalizeDue accepts a bare date (YYYY-MM-DD) or a full RFC3339 timestamp and
// returns an RFC3339 string. A bare date is anchored at UTC midnight. Anything
// else is passed through verbatim — the API owns the contract (design 303
// §due-is-a-date): the API drops the time part on write, the tool does not
// reject it, and --json echoes what actually landed so the loss is visible.
func normalizeDue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s + "T00:00:00.000Z"
	}
	return s
}

// assigned reports whether the task carries a non-empty assignmentInfo — a task
// that originates from Docs / Chat, whose deletion cascades to the source
// (design 303 §architecture-tension-3).
func (t task) assigned() bool {
	return t.AssignmentInfo != nil && len(*t.AssignmentInfo) > 0
}

// splitIDs splits every multi-id arg on whitespace and drops empties, so ids
// pasted into one arg or carrying trailing \r from a pipeline all resolve.
func splitIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("tasks: no valid task ids")
	}
	return ids, nil
}
