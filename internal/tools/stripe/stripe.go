// Package stripe is the built-in Stripe service: a non-interactive cobra tree
// over the Stripe REST surface (https://api.stripe.com/v1). It is a
// read-mostly finance/revenue-ops colleague — reporting plus a few well-scoped
// support mutations (issue a refund, draft/send an invoice, cancel a
// subscription) — not a checkout integration, so it wraps no PaymentIntent
// confirmation, card tokenization, or webhook plumbing.
//
// Auth is "Authorization: Bearer <token>" (the OAuth access token; equivalent
// to Stripe's documented "-u <token>:" Basic form). Every call pins the
// Stripe-Version header so response shapes do not drift. Request bodies for
// mutations are application/x-www-form-urlencoded (Stripe's wire format);
// create/refund verbs forward --idempotency-key as the Idempotency-Key header.
// Stripe errors are a non-2xx status with a JSON body carrying
// error.{type,code,message,param}; a 401 rejects the credential.
package stripe

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

// DefaultBaseURL is the production Stripe API base (already carries /v1).
const DefaultBaseURL = "https://api.stripe.com/v1"

// stripeVersion is the pinned Stripe-Version header sent on every call, so
// response shapes stay stable under us. This is a constant, not a credential.
const stripeVersion = "2026-06-24.dahlia"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/stripe.json).
const EnvAccessToken = "STRIPE_ACCESS_TOKEN"

// Service implements the built-in Stripe tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Stripe API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one stripe subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid --param, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Stripe
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "STRIPE_ACCESS_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. search / get are
// top-level (cross-resource); everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "stripe",
		Short:         "Stripe built-in service (read-mostly finance/revenue-ops)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (Stripe responses are always JSON; accepted for uniformity)")

	root.AddCommand(
		s.newBalanceCmd(token),
		s.newListGetGroup(token, "charge", "Inspect payments (charges)", "/charges"),
		s.newListGetGroup(token, "payment-intent", "Inspect payment intents (read-only)", "/payment_intents"),
		s.newCustomerCmd(token),
		s.newInvoiceCmd(token),
		s.newSubscriptionCmd(token),
		s.newRefundCmd(token),
		s.newListGetGroup(token, "payout", "Settlement reporting (payouts)", "/payouts"),
		s.newListGetGroup(token, "product", "Catalog lookups (products)", "/products"),
		s.newListGetGroup(token, "price", "Catalog lookups (prices)", "/prices"),
		s.newListGetGroup(token, "dispute", "Chargeback triage (disputes)", "/disputes"),
		s.newListGetGroup(token, "event", "Audit trail (events)", "/events"),
		s.newSearchCmd(token),
		s.newGetCmd(token),
	)
	return root
}

// sideEffectAnnotation is the design-318 fact the approval gate reads: "true"
// for verbs that mutate provider state (create/update/cancel/refund/finalize/
// send), "false" for reads. lintServiceTree requires exactly one on every
// runnable leaf and none on group commands, so every leaf factory sets it.
const sideEffectAnnotation = "anycli.side_effect"

// sideEffect returns a fresh annotation map so no two commands share (and could
// mutate) one map.
func sideEffect(mutates bool) map[string]string {
	v := "false"
	if mutates {
		v = "true"
	}
	return map[string]string{sideEffectAnnotation: v}
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
