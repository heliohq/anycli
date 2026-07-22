// Package mercury is the built-in Mercury service: a read-first cobra tree over
// the Mercury Banking API (https://api.mercury.com/api/v1). It exposes the nouns
// an AI finance teammate reasons over — accounts, transactions, recipients,
// treasury, and cards — and normalizes every response into a provider-neutral
// {"data": ...} envelope (a JSON array for list verbs, a JSON object for get
// verbs) so an agent can consume results uniformly.
//
// Auth is a plain OAuth2 Bearer access token (Authorization: Bearer <token>),
// injected as MERCURY_ACCESS_TOKEN by the credential binding
// (definitions/tools/mercury.json). Mercury OAuth tokens do NOT carry the
// "secret-token:" prefix that self-serve static API tokens use.
//
// Output is always JSON. The persistent --json flag controls the ERROR envelope
// format (a structured {"error": {...}} on stderr under --json, plain text
// otherwise); data on stdout is always the normalized {"data": ...} envelope.
//
// This first pass is read-only: money-movement writes (send money, internal
// transfer, recipient create/update) are deliberately deferred behind a
// stage-1 review of Mercury's idempotency-key and approval-request semantics.
package mercury

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

// DefaultBaseURL is the production Mercury Banking API base (path carries /api/v1).
const DefaultBaseURL = "https://api.mercury.com/api/v1"

// EnvToken is the env var the credential binding injects (definitions/tools/mercury.json).
const EnvToken = "MERCURY_ACCESS_TOKEN"

// Service implements the built-in Mercury tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Mercury API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mercury subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (unknown subcommands, bad flags,
// missing required flags) are exit 2; runtime/API errors (Mercury non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &apiError{msg: "MERCURY_ACCESS_TOKEN is not set"})
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
	// usageError plus every cobra-originated parse/arg/unknown-command error
	// is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mercury",
		Short:         "Mercury built-in service (banking accounts, transactions, recipients, treasury, cards)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON error output on stderr (stdout data is always JSON)")

	account := newGroupCmd("account", "List and inspect accounts")
	account.AddCommand(
		s.newAccountListCmd(token),
		s.newAccountGetCmd(token),
	)
	transaction := newGroupCmd("transaction", "List and inspect account transactions")
	transaction.AddCommand(
		s.newTransactionListCmd(token),
		s.newTransactionGetCmd(token),
	)
	recipient := newGroupCmd("recipient", "List and inspect payment recipients")
	recipient.AddCommand(
		s.newRecipientListCmd(token),
		s.newRecipientGetCmd(token),
	)
	treasury := newGroupCmd("treasury", "Inspect treasury (money-market / T-bill) accounts")
	treasury.AddCommand(
		s.newTreasuryGetCmd(token),
	)
	card := newGroupCmd("card", "List the cards on an account")
	card.AddCommand(
		s.newCardListCmd(token),
	)

	root.AddCommand(account, transaction, recipient, treasury, card)
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
