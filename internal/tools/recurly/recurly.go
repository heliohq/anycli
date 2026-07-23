// Package recurly is the built-in Recurly service: a read-first, resource-grouped
// cobra tree over the Recurly V3 REST API (https://recurly.com/developers/api).
// Recurly is a subscription-billing platform; the tree exposes account,
// subscription, invoice, transaction, plan, coupon, line-item, and site lookups
// plus a curated set of subscription/invoice lifecycle writes. Auth is HTTP
// Basic with a private API key as the username and a blank password; every
// request pins the API version through the Accept header. IDs accept Recurly's
// human-friendly alias prefixes verbatim (code-<account_code>,
// number-<invoice_number>, uuid-<subscription_uuid>).
package recurly

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

// Service implements the built-in Recurly tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the host; empty = the region-selected Recurly host.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one recurly subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required args, unknown subcommands) are exit 2; runtime/API errors (Recurly
// non-2xx, transport failure) are exit 1. Errors render to stderr — JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvKey]
	if key == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key, env[EnvRegion])
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return execution.Failure(err), nil
	}
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse missing-key check).
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

// emitJSON writes a JSON body to stdout verbatim (with a trailing newline).
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// newRoot builds the resource-grouped cobra tree.
func (s *Service) newRoot(key, region string) *cobra.Command {
	root := &cobra.Command{
		Use:           "recurly",
		Short:         "Recurly subscription-billing built-in service (V3 REST API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newAccountGroup(key, region),
		s.newSubscriptionGroup(key, region),
		s.newInvoiceGroup(key, region),
		s.newTransactionGroup(key, region),
		s.newPlanGroup(key, region),
		s.newCouponGroup(key, region),
		s.newLineItemGroup(key, region),
		s.newSiteGroup(key, region),
	)
	return root
}

// newGroupCmd is a runnable command group: cobra skips Args validation on
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

// NewCommandTree returns the full tree built with empty credentials for dry-run
// parsing/traversal (tools.Service seam). The credentials are only captured by
// RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("", "") }
