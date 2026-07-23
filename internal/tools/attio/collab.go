package attio

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// --- notes ----------------------------------------------------------------

// newNoteListCmd is `note list [--record <object>:<id>]` (GET /v2/notes):
// list notes, optionally filtered to one parent record via parent_object /
// parent_record_id query params.
func (s *Service) newNoteListCmd(token string) *cobra.Command {
	var record string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List notes (optionally filtered to a record)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&record, "record", "", "filter to a parent record: <object>:<record_id>")
	lo := registerLimitOffset(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if strings.TrimSpace(record) != "" {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			q.Set("parent_object", object)
			q.Set("parent_record_id", recordID)
		}
		lo.applyToQuery(q)
		path := "/v2/notes"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newNoteGetCmd is `note get <note_id>` (GET /v2/notes/{note_id}).
func (s *Service) newNoteGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <note_id>",
		Short:       "Get one note by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/notes/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newNoteCreateCmd is `note create --parent <object>:<id> --title <t>
// (--markdown <md> | --plaintext <txt>)` (POST /v2/notes): log a note. Exactly
// one of --markdown / --plaintext selects the content and the format field.
func (s *Service) newNoteCreateCmd(token string) *cobra.Command {
	var parent, title, markdown, plaintext string
	cmd := &cobra.Command{
		Use:         "create --parent <object>:<id> --title <t> (--markdown <md> | --plaintext <txt>)",
		Short:       "Create a note on a record",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent record: <object>:<record_id> (required)")
	cmd.Flags().StringVar(&title, "title", "", "note title (plaintext, required)")
	cmd.Flags().StringVar(&markdown, "markdown", "", "note content as markdown")
	cmd.Flags().StringVar(&plaintext, "plaintext", "", "note content as plaintext")
	_ = cmd.MarkFlagRequired("parent")
	_ = cmd.MarkFlagRequired("title")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		object, recordID, err := parseRecordRef("parent", parent)
		if err != nil {
			return err
		}
		mdSet := cmd.Flags().Changed("markdown")
		txtSet := cmd.Flags().Changed("plaintext")
		if mdSet == txtSet {
			return &usageError{msg: "pass exactly one of --markdown or --plaintext"}
		}
		format, content := "markdown", markdown
		if txtSet {
			format, content = "plaintext", plaintext
		}
		payload := map[string]any{"data": map[string]any{
			"parent_object":    object,
			"parent_record_id": recordID,
			"title":            title,
			"format":           format,
			"content":          content,
		}}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/notes", payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newNoteDeleteCmd is `note delete <note_id>` (DELETE /v2/notes/{note_id}).
func (s *Service) newNoteDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <note_id>",
		Short:       "Delete one note by id",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodDelete, "/v2/notes/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// --- tasks ----------------------------------------------------------------

// newTaskListCmd is `task list [--record <object>:<id>]` (GET /v2/tasks):
// list tasks, optionally scoped to a linked record (linked_object /
// linked_record_id query params).
func (s *Service) newTaskListCmd(token string) *cobra.Command {
	var record string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tasks (optionally filtered to a linked record)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&record, "record", "", "filter to a linked record: <object>:<record_id>")
	lo := registerLimitOffset(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if strings.TrimSpace(record) != "" {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			q.Set("linked_object", object)
			q.Set("linked_record_id", recordID)
		}
		lo.applyToQuery(q)
		path := "/v2/tasks"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newTaskGetCmd is `task get <task_id>` (GET /v2/tasks/{task_id}).
func (s *Service) newTaskGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <task_id>",
		Short:       "Get one task by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tasks/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newTaskCreateCmd is `task create --content <txt>` (POST /v2/tasks). The API
// requires content, format, deadline_at, is_completed, linked_records and
// assignees on every create, so the CLI always sends them: format plaintext,
// is_completed false, deadline_at null unless --deadline, and empty
// linked_records / assignees unless --record / --assignee.
func (s *Service) newTaskCreateCmd(token string) *cobra.Command {
	var content, deadline, assignee, record string
	cmd := &cobra.Command{
		Use:         "create --content <txt>",
		Short:       "Create a follow-up task",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&content, "content", "", "task text (plaintext, required)")
	cmd.Flags().StringVar(&deadline, "deadline", "", "deadline as an ISO 8601 timestamp")
	cmd.Flags().StringVar(&assignee, "assignee", "", "workspace member id to assign")
	cmd.Flags().StringVar(&record, "record", "", "linked record: <object>:<record_id>")
	_ = cmd.MarkFlagRequired("content")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data := map[string]any{
			"content":        content,
			"format":         "plaintext",
			"is_completed":   false,
			"linked_records": []any{},
			"assignees":      []any{},
		}
		if strings.TrimSpace(deadline) != "" {
			data["deadline_at"] = deadline
		} else {
			data["deadline_at"] = nil
		}
		if strings.TrimSpace(assignee) != "" {
			data["assignees"] = []any{map[string]any{
				"referenced_actor_type": "workspace-member",
				"referenced_actor_id":   assignee,
			}}
		}
		if strings.TrimSpace(record) != "" {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			data["linked_records"] = []any{map[string]any{
				"target_object":    object,
				"target_record_id": recordID,
			}}
		}
		payload := map[string]any{"data": data}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/tasks", payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newTaskUpdateCmd is `task update <task_id>` (PATCH /v2/tasks/{task_id}). Only
// the changed mutable fields are sent; at least one is required.
func (s *Service) newTaskUpdateCmd(token string) *cobra.Command {
	var completed, deadline, assignee, record string
	cmd := &cobra.Command{
		Use:         "update <task_id>",
		Short:       "Update a task (completion, deadline, assignee or linked record)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&completed, "completed", "", "completion state: true|false")
	cmd.Flags().StringVar(&deadline, "deadline", "", "deadline as an ISO 8601 timestamp")
	cmd.Flags().StringVar(&assignee, "assignee", "", "workspace member id to assign")
	cmd.Flags().StringVar(&record, "record", "", "linked record: <object>:<record_id>")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		data := map[string]any{}
		if cmd.Flags().Changed("completed") {
			switch completed {
			case "true":
				data["is_completed"] = true
			case "false":
				data["is_completed"] = false
			default:
				return &usageError{msg: "--completed must be true or false"}
			}
		}
		if cmd.Flags().Changed("deadline") {
			data["deadline_at"] = deadline
		}
		if cmd.Flags().Changed("assignee") {
			data["assignees"] = []any{map[string]any{
				"referenced_actor_type": "workspace-member",
				"referenced_actor_id":   assignee,
			}}
		}
		if cmd.Flags().Changed("record") {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			data["linked_records"] = []any{map[string]any{
				"target_object":    object,
				"target_record_id": recordID,
			}}
		}
		if len(data) == 0 {
			return &usageError{msg: "task update requires at least one of --completed, --deadline, --assignee, --record"}
		}
		payload := map[string]any{"data": data}
		body, err := s.call(cmd.Context(), token, http.MethodPatch, "/v2/tasks/"+url.PathEscape(args[0]), payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newTaskDeleteCmd is `task delete <task_id>` (DELETE /v2/tasks/{task_id}).
func (s *Service) newTaskDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <task_id>",
		Short:       "Delete one task by id",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodDelete, "/v2/tasks/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// --- threads --------------------------------------------------------------

// newThreadListCmd is `thread list [--record <object>:<id>]` (GET /v2/threads):
// list comment threads, optionally scoped to a record (object + record_id).
func (s *Service) newThreadListCmd(token string) *cobra.Command {
	var record string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List comment threads (optionally filtered to a record)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&record, "record", "", "filter to a record: <object>:<record_id>")
	lo := registerLimitOffset(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if strings.TrimSpace(record) != "" {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			q.Set("object", object)
			q.Set("record_id", recordID)
		}
		lo.applyToQuery(q)
		path := "/v2/threads"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newThreadGetCmd is `thread get <thread_id>` (GET /v2/threads/{thread_id}).
func (s *Service) newThreadGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <thread_id>",
		Short:       "Get one thread (with its comments) by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/threads/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// --- comments -------------------------------------------------------------

// newCommentCreateCmd is `comment create (--thread <id> | --record <object>:<id>)
// --content <txt>` (POST /v2/comments). Every comment requires format
// (plaintext only), content and author; the target is either an existing thread
// or a record (which starts a new thread). author defaults to the connection's
// authorized_by_workspace_member_id (GET /v2/self); --author overrides. If no
// member id can be resolved, the command fails fast requiring --author.
func (s *Service) newCommentCreateCmd(token string) *cobra.Command {
	var thread, record, content, author string
	cmd := &cobra.Command{
		Use:         "create (--thread <id> | --record <object>:<id>) --content <txt>",
		Short:       "Add a comment to a thread or record",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&thread, "thread", "", "existing thread id to reply to")
	cmd.Flags().StringVar(&record, "record", "", "record to comment on (starts a thread): <object>:<record_id>")
	cmd.Flags().StringVar(&content, "content", "", "comment text (plaintext, required)")
	cmd.Flags().StringVar(&author, "author", "", "authoring workspace member id (defaults to the token's member)")
	_ = cmd.MarkFlagRequired("content")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		threadSet := cmd.Flags().Changed("thread")
		recordSet := cmd.Flags().Changed("record")
		if threadSet == recordSet {
			return &usageError{msg: "pass exactly one of --thread or --record"}
		}
		authorID := strings.TrimSpace(author)
		if authorID == "" {
			id, _, err := s.self(cmd.Context(), token)
			if err != nil {
				return err
			}
			authorID = id.AuthorizedByWorkspaceMemberID
			if authorID == "" {
				return &usageError{msg: "cannot resolve an author: this token is not tied to a workspace member — pass --author <member_id>"}
			}
		}
		data := map[string]any{
			"format":  "plaintext",
			"content": content,
			"author":  map[string]any{"type": "workspace-member", "id": authorID},
		}
		if threadSet {
			data["thread_id"] = thread
		} else {
			object, recordID, err := parseRecordRef("record", record)
			if err != nil {
				return err
			}
			data["record"] = map[string]any{"object": object, "record_id": recordID}
		}
		payload := map[string]any{"data": data}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/comments", payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newCommentGetCmd is `comment get <comment_id>` (GET /v2/comments/{comment_id}).
func (s *Service) newCommentGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <comment_id>",
		Short:       "Get one comment by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/comments/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newCommentDeleteCmd is `comment delete <comment_id>`
// (DELETE /v2/comments/{comment_id}).
func (s *Service) newCommentDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <comment_id>",
		Short:       "Delete one comment by id",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodDelete, "/v2/comments/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}
