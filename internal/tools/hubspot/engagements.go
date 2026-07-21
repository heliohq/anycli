package hubspot

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newNoteGroup builds the note engagement group. get/list/update/delete reuse
// the generic object verbs (object type "notes"); create is specialized to map
// --body/--owner/--timestamp to note properties and the id flags to inline
// associations.
func (s *Service) newNoteGroup(token string) *cobra.Command {
	group := newGroupCmd("note", "Manage notes")
	group.AddCommand(
		s.newNoteCreateCmd(token),
		s.newObjectGetCmd(token, "note", "notes"),
		s.newObjectListCmd(token, "notes"),
		s.newObjectUpdateCmd(token, "notes"),
		s.newObjectDeleteCmd(token, "notes"),
	)
	return group
}

func (s *Service) newNoteCreateCmd(token string) *cobra.Command {
	var body, owner, timestamp string
	var props []string
	var assoc engagementAssoc
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a note, optionally associated to records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			properties, err := parseProps(props)
			if err != nil {
				return err
			}
			if properties == nil {
				properties = map[string]string{}
			}
			properties["hs_timestamp"] = resolveTimestamp(timestamp)
			if body != "" {
				properties["hs_note_body"] = body
			}
			if owner != "" {
				properties["hubspot_owner_id"] = owner
			}
			payload := map[string]any{"properties": properties}
			if entries := buildEngagementAssociations("notes", assoc); len(entries) > 0 {
				payload["associations"] = entries
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, objectPathBase("notes"), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "note body (hs_note_body)")
	cmd.Flags().StringVar(&owner, "owner", "", "owner id (hubspot_owner_id)")
	cmd.Flags().StringVar(&timestamp, "timestamp", "", "note timestamp (hs_timestamp); defaults to now")
	cmd.Flags().StringArrayVar(&props, "prop", nil, "extra property key=value (repeatable)")
	registerAssocFlags(cmd, &assoc)
	return cmd
}

// newTaskGroup builds the task engagement group. get/list/update/delete reuse
// the generic object verbs; create maps the task fields, and complete is a
// PATCH shortcut that sets hs_task_status=COMPLETED.
func (s *Service) newTaskGroup(token string) *cobra.Command {
	group := newGroupCmd("task", "Manage tasks")
	group.AddCommand(
		s.newTaskCreateCmd(token),
		s.newObjectGetCmd(token, "task", "tasks"),
		s.newObjectListCmd(token, "tasks"),
		s.newObjectUpdateCmd(token, "tasks"),
		s.newTaskCompleteCmd(token),
		s.newObjectDeleteCmd(token, "tasks"),
	)
	return group
}

func (s *Service) newTaskCreateCmd(token string) *cobra.Command {
	var subject, body, due, owner, status, priority string
	var props []string
	var assoc engagementAssoc
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task, optionally associated to records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			properties, err := parseProps(props)
			if err != nil {
				return err
			}
			if properties == nil {
				properties = map[string]string{}
			}
			properties["hs_timestamp"] = resolveTimestamp(due)
			if subject != "" {
				properties["hs_task_subject"] = subject
			}
			if body != "" {
				properties["hs_task_body"] = body
			}
			if owner != "" {
				properties["hubspot_owner_id"] = owner
			}
			if status != "" {
				properties["hs_task_status"] = status
			}
			if priority != "" {
				properties["hs_task_priority"] = priority
			}
			payload := map[string]any{"properties": properties}
			if entries := buildEngagementAssociations("tasks", assoc); len(entries) > 0 {
				payload["associations"] = entries
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, objectPathBase("tasks"), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "task title (hs_task_subject)")
	cmd.Flags().StringVar(&body, "body", "", "task notes (hs_task_body)")
	cmd.Flags().StringVar(&due, "due", "", "due date (hs_timestamp); defaults to now")
	cmd.Flags().StringVar(&owner, "owner", "", "owner id (hubspot_owner_id)")
	cmd.Flags().StringVar(&status, "status", "", "status (hs_task_status): NOT_STARTED|IN_PROGRESS|WAITING|COMPLETED|DEFERRED")
	cmd.Flags().StringVar(&priority, "priority", "", "priority (hs_task_priority): LOW|MEDIUM|HIGH")
	cmd.Flags().StringArrayVar(&props, "prop", nil, "extra property key=value (repeatable)")
	registerAssocFlags(cmd, &assoc)
	return cmd
}

func (s *Service) newTaskCompleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a task completed (hs_task_status=COMPLETED)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"properties": map[string]string{"hs_task_status": "COMPLETED"}}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, objectPathBase("tasks")+"/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
