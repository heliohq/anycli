// Package square is the built-in Square service: a non-interactive cobra tree
// over the Square Connect v2 REST surface (https://connect.squareup.com/v2).
// Auth is "Authorization: Bearer <access_token>" plus a fixed Square-Version
// header on every request. Square fails with a non-2xx status and a JSON body
// carrying an errors[] array ({category, code, detail}); every call surfaces
// that detail. Read verbs (list/get/search) never mutate; write verbs
// (create/update/publish) do — see the anycli.side_effect annotations. Command
// output is the provider JSON on stdout verbatim (passthrough + newline).
package square

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

// DefaultBaseURL is the production Square Connect API host root (paths carry
// their own /v2 prefix so the raw `api` escape hatch takes a full "/v2/..."
// path). The sandbox host is https://connect.squareupsandbox.com — override via
// BaseURL / the SQUARE_BASE_URL env for L2 harness runs against sandbox.
const DefaultBaseURL = "https://connect.squareup.com"

// squareVersion pins the Square-Version header sent by every built-in call.
// Square dates its API by this header; pinning one date keeps request/response
// shapes stable regardless of the account's default version.
const squareVersion = "2026-07-15"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/square.json). Square OAuth access tokens expire in 30 days;
// the Helio token gateway refreshes them out of band — anycli only sees a live
// bearer token.
const EnvAccessToken = "SQUARE_ACCESS_TOKEN"

// EnvBaseURL optionally overrides the API host (e.g. the sandbox host for L2).
const EnvBaseURL = "SQUARE_BASE_URL"

// Service implements the built-in Square tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Square API host root; empty = DefaultBaseURL (or the
	// SQUARE_BASE_URL env when set). Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one square subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Square
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "SQUARE_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if v := env[EnvBaseURL]; v != "" && s.BaseURL == "" {
		s.BaseURL = v
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

// usageError is a parameter / usage error: bad flag combination, missing
// required flag, invalid JSON, or an unknown subcommand. Exit code 2, kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Square non-2xx response or a transport
// failure. Exit code 1, kind "api". status is the HTTP status (0 for
// transport/network failures). It wraps the underlying cause so errors.As for
// *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "square",
		Short:         "Square built-in service (Connect v2 REST)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	payment := newGroupCmd("payment", "List and retrieve payments")
	payment.AddCommand(s.newPaymentListCmd(token), s.newPaymentGetCmd(token))

	order := newGroupCmd("order", "Search and retrieve orders")
	order.AddCommand(s.newOrderSearchCmd(token), s.newOrderGetCmd(token))

	customer := newGroupCmd("customer", "Manage customer profiles")
	customer.AddCommand(
		s.newCustomerListCmd(token),
		s.newCustomerSearchCmd(token),
		s.newCustomerGetCmd(token),
		s.newCustomerCreateCmd(token),
		s.newCustomerUpdateCmd(token),
	)

	catalog := newGroupCmd("catalog", "Browse the product catalog")
	catalog.AddCommand(
		s.newCatalogListCmd(token),
		s.newCatalogSearchCmd(token),
		s.newCatalogGetCmd(token),
	)

	invoice := newGroupCmd("invoice", "Draft, list, and publish invoices")
	invoice.AddCommand(
		s.newInvoiceListCmd(token),
		s.newInvoiceSearchCmd(token),
		s.newInvoiceGetCmd(token),
		s.newInvoiceCreateCmd(token),
		s.newInvoicePublishCmd(token),
	)

	inventory := newGroupCmd("inventory", "Read stock levels")
	inventory.AddCommand(s.newInventoryGetCmd(token))

	location := newGroupCmd("location", "Resolve seller locations")
	location.AddCommand(s.newLocationListCmd(token), s.newLocationGetCmd(token))

	root.AddCommand(
		payment, order, customer, catalog, invoice, inventory, location,
		s.newAPICmd(token),
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
