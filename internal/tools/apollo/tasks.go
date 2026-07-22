package apollo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTasksCmd builds the `tasks` group: create follow-up tasks against contacts
// and list them.
func (s *Service) newTasksCmd(token string) *cobra.Command {
	cmd := newGroupCmd("tasks", "Manage follow-up tasks")
	cmd.AddCommand(
		s.newTasksCreateCmd(token),
		s.newTasksSearchCmd(token),
	)
	return cmd
}

// newTasksCreateCmd wraps POST /tasks/bulk_create. Apollo's task create is a
// bulk endpoint: --contact-id (repeatable) supplies the targets; type / due /
// priority describe the task applied to all of them.
func (s *Service) newTasksCreateCmd(token string) *cobra.Command {
	var body, taskType, dueAt, priority, note string
	var contactIDs []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create follow-up tasks against contacts (POST /tasks/bulk_create)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStrSlice(b, "contact_ids", contactIDs)
			setStr(b, "type", taskType)
			setStr(b, "due_at", dueAt)
			setStr(b, "priority", priority)
			setStr(b, "note", note)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tasks/bulk_create", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&contactIDs, "contact-id", nil, "target contact id (repeatable)")
	cmd.Flags().StringVar(&taskType, "type", "", "task type, e.g. call|action_item|email")
	cmd.Flags().StringVar(&dueAt, "due-at", "", "ISO-8601 due timestamp")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: high|medium|low")
	cmd.Flags().StringVar(&note, "note", "", "task note/body")
	registerBodyFlag(cmd, &body)
	_ = cmd.MarkFlagRequired("contact-id")
	return cmd
}

// newTasksSearchCmd wraps POST /tasks/search.
func (s *Service) newTasksSearchCmd(token string) *cobra.Command {
	var body string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "List tasks (POST /tasks/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tasks/search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}
