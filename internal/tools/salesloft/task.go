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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
