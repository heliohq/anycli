package outreach

import (
	"net/url"

	"github.com/spf13/cobra"
)

var taskResource = resource{path: "tasks", typ: "task"}

// longTaskGet sits here because the leaf comes from the shared get constructor.
const longTaskGet = "One task with its action type, due date, state and hoisted `prospect_id`\n" +
	"and `owner_id`. `state` decides whether `task complete` or `task snooze`\n" +
	"still means anything."

// newTaskCmd builds the task resource group — the teammate works the task queue.
func (s *Service) newTaskCmd(token string) *cobra.Command {
	group := newGroupCmd("task", "Work the task queue")
	group.AddCommand(
		s.newTaskListCmd(token),
		s.newGetCmd(token, taskResource, longTaskGet),
		s.newTaskCreateCmd(token),
		s.newTaskCompleteCmd(token),
		s.newTaskSnoozeCmd(token),
	)
	return group
}

func (s *Service) newTaskListCmd(token string) *cobra.Command {
	var prospectID, ownerID, state string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (one page)",
		Long: "--state is the filter that matters: `incomplete` is the working queue, and\n" +
			"without it completed tasks come back mixed in. --owner-id scopes to one\n" +
			"seat's queue and takes a user id from `user list`, not an email.",
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
		Use:   "create",
		Short: "Create a task",
		Long: "--action is the task type (call, email, general), --due takes an ISO-8601\n" +
			"timestamp and sets `dueAt`, --prospect-id links it to a person and\n" +
			"--owner-id assigns it to a seat. Without --owner-id the task is created\n" +
			"unowned and appears in nobody's queue. Anything else goes through --attr.",
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
		Use:   "complete <id>",
		Short: "Mark a task complete (markComplete action)",
		Long: "Runs Outreach's markComplete action rather than patching a state attribute,\n" +
			"which is why there is no way to complete a task through `task create`-style\n" +
			"fields. --note is recorded as the completion note on the resulting\n" +
			"activity. Nothing here re-opens a completed task.",
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
		Use:   "snooze <id>",
		Short: "Snooze a task (snooze action)",
		Long: "Parameters pass through raw: `--param snoozeUntil=2026-08-01T00:00:00Z`\n" +
			"becomes actionParams[snoozeUntil]. Nothing validates the names locally, so\n" +
			"a misspelled one is accepted here and rejected by Outreach. Snoozing moves\n" +
			"the due date; the task stays in the queue.",
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
