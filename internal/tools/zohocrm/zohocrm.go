// Package zohocrm is the built-in Zoho CRM service: a resource-grouped cobra
// tree over the Zoho CRM REST API v8 (https://www.zohoapis.com/crm/v8). It
// wraps records (list/get/create/update/delete/search), COQL queries, notes,
// and settings/user/org metadata so an assistant can look people up, capture
// leads, and keep deals current. Zoho fails with a non-2xx status and a JSON
// body carrying code/message — every call surfaces both.
//
// V1 is scoped to Zoho's US datacenter (.com hosts): the access token is
// datacenter-specific, so a non-US Zoho account fails at the token layer with
// an explicit provider error rather than any silent fallback.
package zohocrm

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

// DefaultBaseURL is the production Zoho CRM US-datacenter API host. Paths add
// the /crm/v8 version prefix; tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://www.zohoapis.com"

// apiPrefix is the versioned path prefix on every CRM call.
const apiPrefix = "/crm/v8"

// EnvToken is the env var the credential binding injects
// (definitions/tools/zoho-crm.json).
const EnvToken = "ZOHO_CRM_ACCESS_TOKEN"

// Service implements the built-in Zoho CRM tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API host; empty = DefaultBaseURL. Tests point it
	// at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one zoho-crm subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Zoho non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
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

// newRoot builds the resource-grouped cobra tree. query and org are top-level;
// everything else hangs under a resource group (record, note, module, field,
// user).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "zoho-crm",
		Short:         "Zoho CRM built-in service (records, COQL, notes, metadata)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output on error")

	record := newGroupCmd("record", "Read and write CRM records")
	record.AddCommand(
		s.newRecordListCmd(token),
		s.newRecordGetCmd(token),
		s.newRecordCreateCmd(token),
		s.newRecordUpdateCmd(token),
		s.newRecordDeleteCmd(token),
		s.newRecordSearchCmd(token),
	)
	note := newGroupCmd("note", "List and add notes on a record")
	note.AddCommand(
		s.newNoteListCmd(token),
		s.newNoteAddCmd(token),
	)
	module := newGroupCmd("module", "Discover modules")
	module.AddCommand(s.newModuleListCmd(token))
	field := newGroupCmd("field", "Discover a module's field API names")
	field.AddCommand(s.newFieldListCmd(token))
	user := newGroupCmd("user", "Look up CRM users")
	user.AddCommand(
		s.newUserListCmd(token),
		s.newUserMeCmd(token),
	)

	root.AddCommand(
		s.newQueryCmd(token),
		s.newOrgGetCmd(token),
		record, note, module, field, user,
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
