package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTaskCmd groups the rep's task queue: list, fetch, create, and complete.
func (s *Service) newTaskCmd(token string) *cobra.Command {
	cmd := newGroupCmd("task", "Manage tasks")
	cmd.AddCommand(
		s.newTaskListCmd(token),
		s.newTaskGetCmd(token),
		s.newTaskCreateCmd(token),
		s.newTaskUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newTaskListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (GET /v2/tasks)",
		Long: "The rep's queue, unfiltered by default and team-wide rather than\n" +
			"personal. There are no named filters, so narrowing goes through\n" +
			"`--filter` — `--filter current_state=scheduled` for what is still open —\n" +
			"combined with `--sort-by due_date` and the shared paging controls. A\n" +
			"task carries the person or account it hangs on, so filtering by those\n" +
			"ids is how you get a single prospect's outstanding work.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/tasks", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newTaskGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch one task (GET /v2/tasks/{id})",
		Long: "`--id` is required. Returns the task's subject, `task_type`, `due_date`,\n" +
			"`current_state` and the person or account it is attached to. Completing\n" +
			"it is `task update --current-state completed`; no dedicated complete\n" +
			"verb exists.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/tasks/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "task id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTaskCreateCmd(token string) *cobra.Command {
	var subject, taskType, dueDate, currentState, body string
	var personID, accountID, userID int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task (POST /v2/tasks)",
		Long: "Nothing is required by the CLI, but a usable task needs at least\n" +
			"`--subject` and `--task-type` (`call`, `email`, `general`, `other`).\n" +
			"`--due-date` is ISO-8601 and `--current-state` is `scheduled` or\n" +
			"`completed` — creating one straight into `completed` is how work that\n" +
			"already happened gets logged. `--person-id` or `--account-id` attaches\n" +
			"it to a record, and `--user-id` puts it on another rep's queue instead\n" +
			"of the connected user's. A task is internal: creating one contacts\n" +
			"nobody outside the team.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			named := taskNamedBody(cmd, subject, taskType, dueDate, currentState, personID, accountID, userID)
			payload, err := mergeBody(named, body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tasks", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerTaskWriteFlags(cmd, &subject, &taskType, &dueDate, &currentState, &personID, &accountID, &userID, &body)
	return cmd
}

func (s *Service) newTaskUpdateCmd(token string) *cobra.Command {
	var id, subject, taskType, dueDate, currentState, body string
	var personID, accountID, userID int
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a task (PUT /v2/tasks/{id}); set --current-state completed to finish it",
		Long: "`--id` is required and the write is partial: only the flags passed are\n" +
			"sent, so the state can be changed without restating the subject or due\n" +
			"date. Completing is the whole of the state machine here — there is no\n" +
			"cancel or delete, and the only way back is setting `--current-state`\n" +
			"to `scheduled` again. `--body` overrides the named flags for fields\n" +
			"without one.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			named := taskNamedBody(cmd, subject, taskType, dueDate, currentState, personID, accountID, userID)
			payload, err := mergeBody(named, body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/tasks/"+id, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "task id")
	_ = cmd.MarkFlagRequired("id")
	registerTaskWriteFlags(cmd, &subject, &taskType, &dueDate, &currentState, &personID, &accountID, &userID, &body)
	return cmd
}

// taskNamedBody builds the task body from named flags, omitting empty strings
// and integer ids that were not passed.
func taskNamedBody(cmd *cobra.Command, subject, taskType, dueDate, currentState string, personID, accountID, userID int) map[string]any {
	body := map[string]any{}
	if subject != "" {
		body["subject"] = subject
	}
	if taskType != "" {
		body["task_type"] = taskType
	}
	if dueDate != "" {
		body["due_date"] = dueDate
	}
	if currentState != "" {
		body["current_state"] = currentState
	}
	if cmd.Flags().Changed("person-id") {
		body["person_id"] = personID
	}
	if cmd.Flags().Changed("account-id") {
		body["account_id"] = accountID
	}
	if cmd.Flags().Changed("user-id") {
		body["user_id"] = userID
	}
	return body
}

func registerTaskWriteFlags(cmd *cobra.Command, subject, taskType, dueDate, currentState *string, personID, accountID, userID *int, body *string) {
	cmd.Flags().StringVar(subject, "subject", "", "task subject")
	cmd.Flags().StringVar(taskType, "task-type", "", "task type: call|email|general|other")
	cmd.Flags().StringVar(dueDate, "due-date", "", "due date (ISO-8601)")
	cmd.Flags().StringVar(currentState, "current-state", "", "current state: scheduled|completed")
	cmd.Flags().IntVar(personID, "person-id", 0, "linked person id")
	cmd.Flags().IntVar(accountID, "account-id", 0, "linked account id")
	cmd.Flags().IntVar(userID, "user-id", 0, "assigned user id")
	cmd.Flags().StringVar(body, "body", "", "raw JSON body; keys override the named flags for full fidelity")
}
