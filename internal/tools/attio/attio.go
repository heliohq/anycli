// Package attio is the built-in Attio CRM service: a cobra tree over the
// api.attio.com REST v2 surface. Attio is a data-model-first CRM — a workspace
// holds objects (people, companies, deals, custom …), each object holds
// records, and lists overlay records as pipelines with per-list entries; notes,
// tasks, comments and threads attach to records and entries. Because schemas are
// per-workspace, the tree exposes read-only schema introspection (object,
// attribute) so an agent can build valid write payloads instead of hardcoding
// slugs. Attio fails with a non-2xx status and a JSON body carrying
// status_code/type/code/message — every call surfaces both.
package attio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Attio API base (paths already carry /v2).
const DefaultBaseURL = "https://api.attio.com"

// EnvToken is the env var the credential binding injects (definitions/tools/attio.json).
const EnvToken = "ATTIO_ACCESS_TOKEN"

// Service implements the built-in Attio tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Attio API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one attio subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, invalid JSON,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Attio non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "ATTIO_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// newRoot builds the resource-grouped cobra tree. whoami is top-level;
// everything else hangs under a resource group (object, attribute, record,
// list, entry, note, task, thread, comment, member).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "attio",
		Short:         "Attio CRM built-in service (records, lists, notes, tasks)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	// Global (persistent) flag: --json forces verbatim provider JSON on stdout;
	// without it, commands print a compact human-readable summary.
	root.PersistentFlags().Bool("json", false, "emit the provider's raw JSON response instead of a summary")

	object := newGroupCmd("object", "Discover objects (schema introspection)")
	object.AddCommand(s.newObjectListCmd(token), s.newObjectGetCmd(token))

	attribute := newGroupCmd("attribute", "Discover attributes, select options and statuses")
	attribute.AddCommand(
		s.newAttributeListCmd(token),
		s.newAttributeOptionsCmd(token),
		s.newAttributeStatusesCmd(token),
	)

	record := newGroupCmd("record", "Read and write records")
	record.AddCommand(
		s.newRecordSearchCmd(token),
		s.newRecordQueryCmd(token),
		s.newRecordGetCmd(token),
		s.newRecordDeleteCmd(token),
		s.newRecordCreateCmd(token),
		s.newRecordUpdateCmd(token),
		s.newRecordUpsertCmd(token),
	)

	list := newGroupCmd("list", "Discover lists")
	list.AddCommand(s.newListListCmd(token), s.newListGetCmd(token))

	entry := newGroupCmd("entry", "Work with list entries (pipeline membership)")
	entry.AddCommand(
		s.newEntryQueryCmd(token),
		s.newEntryAddCmd(token),
		s.newEntryGetCmd(token),
		s.newEntryRemoveCmd(token),
		s.newEntryUpdateCmd(token),
	)

	note := newGroupCmd("note", "Log and read notes")
	note.AddCommand(
		s.newNoteListCmd(token),
		s.newNoteGetCmd(token),
		s.newNoteCreateCmd(token),
		s.newNoteDeleteCmd(token),
	)

	task := newGroupCmd("task", "Manage follow-up tasks")
	task.AddCommand(
		s.newTaskListCmd(token),
		s.newTaskGetCmd(token),
		s.newTaskCreateCmd(token),
		s.newTaskUpdateCmd(token),
		s.newTaskDeleteCmd(token),
	)

	thread := newGroupCmd("thread", "Read comment threads")
	thread.AddCommand(s.newThreadListCmd(token), s.newThreadGetCmd(token))

	comment := newGroupCmd("comment", "Participate in record discussions")
	comment.AddCommand(
		s.newCommentCreateCmd(token),
		s.newCommentGetCmd(token),
		s.newCommentDeleteCmd(token),
	)

	member := newGroupCmd("member", "Resolve workspace members (assignees/actors)")
	member.AddCommand(s.newMemberListCmd(token), s.newMemberGetCmd(token))

	root.AddCommand(
		s.newWhoamiCmd(token),
		object, attribute, record, list, entry, note, task, thread, comment, member,
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
