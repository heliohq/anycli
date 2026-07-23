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
		Use:           "pennylane",
		Short:         "Pennylane built-in service (accounting REST API v2)",
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
