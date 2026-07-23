// Package ramp is the built-in Ramp service: a read-only cobra tree over the
// api.ramp.com Developer API (/developer/v1) for a finance / spend-ops
// teammate — transactions, reimbursements, cards, users, departments,
// locations, and connected-business info. Every call sends
// Authorization: Bearer <access_token> (the OAuth partner access token,
// projected by the Helio token gateway). Ramp fails with a non-2xx status and
// a JSON error body; each call surfaces both.
package ramp

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

// DefaultBaseURL is the production Ramp API base. The per-resource paths carry
// their own /developer/v1 prefix, so the base is the bare host.
const DefaultBaseURL = "https://api.ramp.com"

// EnvToken is the env var the credential binding injects
// (definitions/tools/ramp.json).
const EnvToken = "RAMP_ACCESS_TOKEN"

// Service implements the built-in Ramp tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Ramp API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one ramp subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing args, unknown
// subcommands) are exit 2; runtime/API errors (Ramp non-2xx, transport
// failure) are exit 1. Errors render to stderr — JSON under --json, plain text
// otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "RAMP_ACCESS_TOKEN is not set"})
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags.
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

// newRoot builds the grouped-by-resource cobra tree. `get` is the top-level
// raw-GET passthrough; everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "ramp",
		Short:         "Ramp transactions, reimbursements, cards, users, departments, locations, and business (read)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	// Global (persistent) flag. Pagination flags are NOT global; they register
	// locally on list commands only (see registerPaginationFlags).
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	transaction := newGroupCmd("transaction", "Card transaction ledger")
	transaction.AddCommand(
		s.newTransactionListCmd(token),
		s.newTransactionGetCmd(token),
	)
	reimbursement := newGroupCmd("reimbursement", "Out-of-pocket reimbursements")
	reimbursement.AddCommand(
		s.newReimbursementListCmd(token),
		s.newReimbursementGetCmd(token),
	)
	card := newGroupCmd("card", "Issued virtual and physical cards")
	card.AddCommand(
		s.newCardVirtualCmd(token),
		s.newCardPhysicalCmd(token),
	)
	user := newGroupCmd("user", "Users and cardholders")
	user.AddCommand(
		s.newUserListCmd(token),
		s.newUserGetCmd(token),
	)
	department := newGroupCmd("department", "Department dimension lookups")
	department.AddCommand(
		s.newDepartmentListCmd(token),
		s.newDepartmentGetCmd(token),
	)
	location := newGroupCmd("location", "Location dimension lookups")
	location.AddCommand(
		s.newLocationListCmd(token),
		s.newLocationGetCmd(token),
	)
	business := newGroupCmd("business", "Connected-business info and balance")
	business.AddCommand(
		s.newBusinessInfoCmd(token),
		s.newBusinessBalanceCmd(token),
	)

	root.AddCommand(
		s.newGetCmd(token),
		transaction, reimbursement, card, user, department, location, business,
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
