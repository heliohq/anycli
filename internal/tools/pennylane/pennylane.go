// Package pennylane is the built-in Pennylane service: a resource-grouped cobra
// tree over the Pennylane external REST API v2 (base app.pennylane.com/api/external/v2).
// It accepts an OAuth 2.0 access token (gateway-refreshed) and injects it as a
// Bearer credential, passing the provider's JSON response bodies straight
// through. Pennylane fails with a non-2xx status and a JSON body carrying a
// message; every call surfaces both.
package pennylane

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

// DefaultBaseURL is the production Pennylane external API base (already carries
// /api/external/v2). Tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://app.pennylane.com/api/external/v2"

// EnvToken is the env var the credential binding injects
// (definitions/tools/pennylane.json). The service reads it and sets
// Authorization: Bearer <token>.
const EnvToken = "PENNYLANE_ACCESS_TOKEN"

// Service implements the built-in Pennylane tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Pennylane API base; empty = DefaultBaseURL.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one pennylane subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, invalid JSON,
// missing required flags/args, unknown subcommands) are exit 2; runtime/API
// errors (Pennylane non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "PENNYLANE_ACCESS_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. Every resource hangs under
// a runnable group; list/get are read-only, create/categorize are the only
// mutating leaves.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "pennylane",
		Short: "Pennylane built-in service (accounting REST API v2)",
		Long: "A connection is bound to ONE Pennylane company, so every command operates\n" +
			"on that company's books. The surface is read-heavy: `list` and `get` cover\n" +
			"customers, suppliers, invoices, products, transactions and the ledger,\n" +
			"while the only mutating leaves are `customer create`, `customer-invoice\n" +
			"create` and `transaction categorize`.\n" +
			"\n" +
			"Pagination is cursor-based and nothing here auto-loops. A `list` returns\n" +
			"one page plus a cursor in its metadata; pass it back through `--cursor`\n" +
			"for the next page. `--limit` sizes a page (1-100), `--sort` orders (`-id`\n" +
			"for newest first) and `--filter` takes Pennylane's own filter expression\n" +
			"grammar verbatim — this tool invents no query DSL of its own.\n" +
			"\n" +
			"Request bodies are raw JSON in `--body`, which is required on every\n" +
			"mutating command and may be an object or an array depending on the\n" +
			"endpoint. The global `--json` is an output and error format flag, not a\n" +
			"payload flag; an empty or malformed `--body` is a usage error and sends\n" +
			"nothing.\n" +
			"\n" +
			"The connection requests read-only ledger scopes, so the whole `ledger`\n" +
			"group is read-only by design. A 403 on a write means the scope was never\n" +
			"granted rather than a transient failure, and retrying will not clear it.\n" +
			"\n" +
			"Success prints Pennylane's JSON response body verbatim. Errors render as a\n" +
			"plain message, or as an error envelope under `--json`.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output for errors")

	root.AddCommand(
		s.newCustomerCmd(token),
		s.newSupplierCmd(token),
		s.newCustomerInvoiceCmd(token),
		s.newSupplierInvoiceCmd(token),
		s.newProductCmd(token),
		s.newTransactionCmd(token),
		s.newLedgerCmd(token),
	)
	return root
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
