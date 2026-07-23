package outreach

import (
	"net/url"

	"github.com/spf13/cobra"
)

var taskResource = resource{path: "tasks", typ: "task"}

// newTaskCmd builds the task resource group — the teammate works the task queue.
func (s *Service) newTaskCmd(token string) *cobra.Command {
	group := newGroupCmd("task", "Work the task queue")
	group.AddCommand(
		s.newTaskListCmd(token),
		s.newGetCmd(token, taskResource),
		s.newTaskCreateCmd(token),
		s.newTaskCompleteCmd(token),
		s.newTaskSnoozeCmd(token),
	)
	return group
}

func (s *Service) newTaskListCmd(token string) *cobra.Command {
	var prospectID, ownerID, state string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tasks (one page)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setRelFilter(query, "prospect", prospectID)
			setRelFilter(query, "owner", ownerID)
			setFilter(query, "state", state)
			if err := listFlagsFrom(cmd).apply(query, taskResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, taskResource, query)
		},
	}
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "filter by prospect id")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "filter by owner (user) id")
	cmd.Flags().StringVar(&state, "state", "", "filter by task state (e.g. incomplete, completed)")
	bindListFlags(cmd)
	return cmd
}

func (s *Service) newTaskCreateCmd(token string) *cobra.Command {
	var due, note, action, prospectID, ownerID string
	var attr []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a task",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			attrs, err := parseAttrs(attr)
			if err != nil {
				return err
			}
			setAttr(attrs, "dueAt", due)
			setAttr(attrs, "note", note)
			setAttr(attrs, "action", action)
			rels := map[string]string{}
			setRel(rels, "prospect", prospectID)
			setRel(rels, "owner", ownerID)
			return s.runCreate(cmd.Context(), token, taskResource, attrs, rels)
		},
	}
	cmd.Flags().StringVar(&due, "due", "", "due timestamp (ISO 8601) — sets the dueAt attribute")
	cmd.Flags().StringVar(&note, "note", "", "task note")
	cmd.Flags().StringVar(&action, "action", "", "task action type (e.g. call, email, general)")
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "related prospect id")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "owner (user) id")
	cmd.Flags().StringArrayVar(&attr, "attr", nil, "additional attribute key=value (repeatable; value parsed as JSON when valid)")
	return cmd
}

// newTaskCompleteCmd marks a task complete via the markComplete action, passing
// an optional completion note as the documented actionParams[completionNote]
// query param (not a JSON body).
func (s *Service) newTaskCompleteCmd(token string) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:         "complete <id>",
		Short:       "Mark a task complete (markComplete action)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			var params url.Values
			if note != "" {
				params = url.Values{"actionParams[completionNote]": {note}}
			}
			return s.runAction(cmd.Context(), token, taskResource, args[0], "markComplete", params)
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "completion note (actionParams[completionNote])")
	return cmd
}

// newTaskSnoozeCmd snoozes a task via the snooze action. Snooze parameters are
// passed through as repeatable --param key=value → actionParams[key]=value; the
// exact param names are the caller's responsibility (see the provider docs).
func (s *Service) newTaskSnoozeCmd(token string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         "snooze <id>",
		Short:       "Snooze a task (snooze action)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			actionParams, err := parseActionParams(params)
			if err != nil {
				return err
			}
			return s.runAction(cmd.Context(), token, taskResource, args[0], "snooze", actionParams)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "snooze action param key=value → actionParams[key] (repeatable)")
	return cmd
}
