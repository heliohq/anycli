// Package brex is the built-in Brex service: a read-mostly cobra tree over the
// api.brex.com REST surface for a finance / spend-ops teammate — accounts,
// transactions, expenses, cards, users, budgets, and dimension lookups.
// Every call sends Authorization: Bearer <access_token> (the OAuth partner
// access token, projected by the Helio token gateway). Brex fails with a
// non-2xx status and a JSON error body; each call surfaces both.
package brex

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

// DefaultBaseURL is the production Brex API base. The per-resource paths carry
// their own /v1 or /v2 prefix, so the base is the bare host.
const DefaultBaseURL = "https://api.brex.com"

// EnvToken is the env var the credential binding injects
// (definitions/tools/brex.json).
const EnvToken = "BREX_ACCESS_TOKEN"

// Service implements the built-in Brex tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Brex API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one brex subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing args, unknown
// subcommands) are exit 2; runtime/API errors (Brex non-2xx, transport
// failure) are exit 1. Errors render to stderr — JSON under --json, plain text
// otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "BREX_ACCESS_TOKEN is not set"})
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
		Use:           "brex",
		Short:         "Brex accounts, transactions, expenses, cards, budgets, and users (read)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	// Global (persistent) flag. Pagination flags are NOT global; they register
	// locally on list commands only (see registerPaginationFlags).
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	account := newGroupCmd("account", "Card and cash account balances")
	account.AddCommand(
		s.newAccountCardCmd(token),
		s.newAccountCashCmd(token),
	)
	transaction := newGroupCmd("transaction", "Card and cash transaction ledgers")
	transaction.AddCommand(
		s.newTransactionCardPrimaryCmd(token),
		s.newTransactionCashCmd(token),
	)
	expense := newGroupCmd("expense", "Expenses and receipts (read)")
	expense.AddCommand(
		s.newExpenseListCmd(token),
		s.newExpenseCardCmd(token),
		s.newExpenseGetCmd(token),
	)
	card := newGroupCmd("card", "Issued cards")
	card.AddCommand(
		s.newCardListCmd(token),
		s.newCardGetCmd(token),
	)
	user := newGroupCmd("user", "Users and cardholders (Team API)")
	user.AddCommand(
		s.newUserListCmd(token),
		s.newUserMeCmd(token),
		s.newUserGetCmd(token),
	)
	budget := newGroupCmd("budget", "Budgets and spend limits")
	budget.AddCommand(
		s.newBudgetListCmd(token),
		s.newBudgetGetCmd(token),
		s.newSpendLimitsCmd(token),
	)
	department := newGroupCmd("department", "Department dimension lookups")
	department.AddCommand(s.newDepartmentListCmd(token))
	location := newGroupCmd("location", "Location dimension lookups")
	location.AddCommand(s.newLocationListCmd(token))

	root.AddCommand(
		s.newGetCmd(token),
		account, transaction, expense, card, user, budget, department, location,
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
