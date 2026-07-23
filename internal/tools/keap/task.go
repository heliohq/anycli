package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTaskCmd(token string) *cobra.Command {
	cmd := newGroupCmd("task", "Tasks (list, get, create, update, delete)")
	cmd.AddCommand(
		s.newTaskListCmd(token),
		s.newTaskGetCmd(token),
		s.newTaskCreateCmd(token),
		s.newTaskUpdateCmd(token),
		s.newTaskDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newTaskListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tasks (GET /v2/tasks)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tasks", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newTaskGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <task-id>",
		Short:       "Get a task (GET /v2/tasks/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tasks/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

// taskBodyFlags holds the convenience field flags shared by create/update.
type taskBodyFlags struct {
	assignedToUserID, title, contactID string
	description, dueTime, priority     string
	taskType, jsonBody                 string
}

func registerTaskBodyFlags(cmd *cobra.Command) *taskBodyFlags {
	f := &taskBodyFlags{}
	cmd.Flags().StringVar(&f.assignedToUserID, "assigned-to-user-id", "", "user id the task is assigned to")
	cmd.Flags().StringVar(&f.title, "title", "", "task title")
	cmd.Flags().StringVar(&f.contactID, "contact-id", "", "associated contact id")
	cmd.Flags().StringVar(&f.description, "description", "", "task description")
	cmd.Flags().StringVar(&f.dueTime, "due-time", "", "due time (ISO-8601)")
	cmd.Flags().StringVar(&f.priority, "priority", "", "priority")
	cmd.Flags().StringVar(&f.taskType, "type", "", "task type")
	cmd.Flags().StringVar(&f.jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return f
}

func (f *taskBodyFlags) build() (map[string]any, error) {
	body := map[string]any{}
	if f.assignedToUserID != "" {
		body["assigned_to_user_id"] = f.assignedToUserID
	}
	if f.title != "" {
		body["title"] = f.title
	}
	if f.contactID != "" {
		body["contact_id"] = f.contactID
	}
	if f.description != "" {
		body["description"] = f.description
	}
	if f.dueTime != "" {
		body["due_time"] = f.dueTime
	}
	if f.priority != "" {
		body["priority"] = f.priority
	}
	if f.taskType != "" {
		body["type"] = f.taskType
	}
	if err := applyJSONBody(body, f.jsonBody); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Service) newTaskCreateCmd(token string) *cobra.Command {
	var f *taskBodyFlags
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a task (POST /v2/tasks)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if _, ok := body["assigned_to_user_id"]; !ok {
				return &usageError{msg: "task create requires --assigned-to-user-id (or assigned_to_user_id in --json-body)"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/tasks", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerTaskBodyFlags(cmd)
	return cmd
}

func (s *Service) newTaskUpdateCmd(token string) *cobra.Command {
	var f *taskBodyFlags
	cmd := &cobra.Command{
		Use:         "update <task-id>",
		Short:       "Update a task (PATCH /v2/tasks/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/v2/tasks/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerTaskBodyFlags(cmd)
	return cmd
}

func (s *Service) newTaskDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <task-id>",
		Short:       "Delete a task (DELETE /v2/tasks/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/v2/tasks/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
