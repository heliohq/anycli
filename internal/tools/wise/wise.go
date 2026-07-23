// Package wise is the built-in Wise service: a non-interactive cobra tree over
// the Wise REST surface (https://api.wise.com). Auth is a personal API token
// passed as "Authorization: Bearer <token>". The tool is scoped to read /
// monitor plus non-committal pricing — money movement and balance statements
// are PSD2/SCA-gated and intentionally out of scope (see DESIGN.md). Wise
// errors are non-2xx with a JSON body carrying errors[].message / message; a
// 401/403 rejects the credential. Every command emits the provider JSON on
// stdout verbatim (passthrough + newline).
package wise

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Wise API base. The sandbox host
// (https://api.wise-sandbox.com) is selected at runtime with --base-url;
// sandbox tokens are not valid against production and vice-versa.
const DefaultBaseURL = "https://api.wise.com"

// EnvAPIToken is the env var the credential binding injects
// (definitions/tools/wise.json). Personal tokens are long-lived until revoked.
const EnvAPIToken = "WISE_API_TOKEN"

// Service implements the built-in Wise tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Wise API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server; the --base-url flag sets it at runtime for
	// prod vs sandbox selection.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one wise subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required flags,
// unknown subcommands) are exit 2; runtime/API errors (Wise non-2xx, transport
// failure) are exit 1. Errors render to stderr — JSON under --json, plain text
// otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "WISE_API_TOKEN is not set"})
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

// baseURL returns the effective Wise API base (BaseURL override or the
// production default), trailing slash trimmed.
func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "wise",
		Short:         "Wise built-in service (read / monitor + non-committal pricing)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")
	pf.String("base-url", "", "override the Wise API base URL (default production; e.g. https://api.wise-sandbox.com for sandbox)")

	// A non-empty --base-url overrides the API base for this invocation. When
	// unset it stays empty so an explicit Service.BaseURL (tests / embedding)
	// or the production default is used.
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if v, _ := cmd.Flags().GetString("base-url"); v != "" {
			s.BaseURL = v
		}
		return nil
	}

	profile := newGroupCmd("profile", "List the token's profiles")
	profile.AddCommand(s.newProfileListCmd(token))

	balance := newGroupCmd("balance", "Read multi-currency balances")
	balance.AddCommand(
		s.newBalanceListCmd(token),
		s.newBalanceGetCmd(token),
	)

	transfer := newGroupCmd("transfer", "Monitor outgoing transfers")
	transfer.AddCommand(
		s.newTransferListCmd(token),
		s.newTransferGetCmd(token),
	)

	activity := newGroupCmd("activity", "Read the account activity feed")
	activity.AddCommand(s.newActivityListCmd(token))

	recipient := newGroupCmd("recipient", "Look up saved recipient accounts")
	recipient.AddCommand(s.newRecipientListCmd(token))

	quote := newGroupCmd("quote", "Price a hypothetical transfer")
	quote.AddCommand(s.newQuoteCreateCmd(token))

	currency := newGroupCmd("currency", "Reference: supported currencies")
	currency.AddCommand(s.newCurrencyListCmd(token))

	root.AddCommand(profile, balance, transfer, activity, recipient, quote, currency)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it.
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
