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

// The generic list/get resource groups are all built by newListGetGroup, so
// their prose cannot live at the constructor. It lives here instead, one pair
// per resource, in the order the root registers them.
const (
	longChargeList = "A charge is the settled record of money that actually moved, one per\n" +
		"successful payment — including payments made through a PaymentIntent, which\n" +
		"is why this, not `payment-intent list`, answers what was collected. Filter\n" +
		"with `--param customer=cus_123` or a `created` range."

	longChargeGet = "Takes a `ch_` id. `refunded` and `amount_refunded` say whether money has\n" +
		"already been returned on this payment — read them before `refund create` so\n" +
		"a second refund is not issued by accident. `balance_transaction` links to\n" +
		"the fee and net that `balance transactions` reports."

	longPaymentIntentList = "A PaymentIntent is the INTENT to collect and may be incomplete, awaiting\n" +
		"customer action, or abandoned; `charge list` is what actually settled. This\n" +
		"surface is read-only here — nothing in this tool confirms, captures or\n" +
		"cancels an intent. Filter with `--param customer=cus_123`."

	longPaymentIntentGet = "Takes a `pi_` id. `status` carries the whole story — succeeded,\n" +
		"requires_payment_method, requires_action, canceled — and `latest_charge` is\n" +
		"the bridge to the settled charge. Read-only: this tool never advances an\n" +
		"intent's state."

	longPayoutList = "Payouts move money from the Stripe balance to the connected bank account —\n" +
		"the settlement side, not customer payments. Filter with\n" +
		"`--param status=paid|pending|failed|in_transit` or an `arrival_date` range\n" +
		"when reconciling a missing deposit."

	longPayoutGet = "Takes a `po_` id. `status` and `failure_code` explain a deposit that never\n" +
		"landed. Which transactions the payout was composed of is a different read:\n" +
		"`balance transactions --param payout=po_123`."

	longProductList = "Products are what is sold and carry NO amounts — money lives on prices, so\n" +
		"this list alone cannot answer what anything costs. `--param active=true`\n" +
		"hides archived products."

	longProductGet = "Takes a `prod_` id and returns the product's own fields. Its prices are a\n" +
		"separate read: `price list --param product=prod_123`."

	longPriceList = "Prices carry the amounts and the billing intervals, and one product can have\n" +
		"many. `--param product=prod_123` scopes to a single product and\n" +
		"`--param active=true` hides retired prices. `unit_amount` is in the smallest\n" +
		"currency unit."

	longPriceGet = "Takes a `price_` id. `unit_amount` is in cents for a decimal currency; a\n" +
		"`recurring` object describes the subscription billing interval, and its\n" +
		"absence means the price is one-off."

	longDisputeList = "Disputes are chargebacks a cardholder raised, and each carries a deadline:\n" +
		"`evidence_details.due_by` is when Stripe stops accepting a response. Filter\n" +
		"with `--param charge=ch_123` or a `created` range. Submitting evidence is\n" +
		"not possible from this tool — this is triage only."

	longDisputeGet = "Takes a `dp_` id. `status`, `reason` and `evidence_details.due_by` are the\n" +
		"triage fields. The disputed amount has already been withheld from the\n" +
		"balance, so this is money at risk rather than money still held."

	longEventList = "The audit trail of what changed in this account and when, each entry\n" +
		"carrying the affected object under `data.object` as it looked at the time.\n" +
		"Filter with `--param type=charge.refunded` or a `created` range. Stripe\n" +
		"keeps events for roughly 30 days, so older history has to be reconstructed\n" +
		"from the objects themselves."

	longEventGet = "Takes an `evt_` id and returns the same payload a webhook would have\n" +
		"delivered, object snapshot included. Reading it here neither acknowledges\n" +
		"nor re-delivers any webhook."
)

// newRoot builds the grouped-by-resource cobra tree. search / get are
// top-level (cross-resource); everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "stripe",
		Short: "Stripe built-in service (read-mostly finance/revenue-ops)",
		Long: "Stripe as a finance, revenue-ops and support colleague rather than a\n" +
			"checkout integration: reporting, plus a few scoped support mutations —\n" +
			"issue a refund, draft and send an invoice, cancel a subscription, maintain\n" +
			"a customer. Nothing here confirms or captures a payment, tokenizes a card,\n" +
			"or touches the webhook bus.\n" +
			"\n" +
			"AMOUNTS ARE IN THE SMALLEST CURRENCY UNIT. `--param amount=500` is $5.00\n" +
			"and `--param amount=5` is five cents. Zero-decimal currencies such as JPY\n" +
			"are the exception and take the whole number.\n" +
			"\n" +
			"--param is the universal escape hatch and maps 1:1 onto Stripe's own\n" +
			"fields: a query filter on a list (`--param customer=cus_123`), a form\n" +
			"field on a write (`--param amount=500`), with bracket notation passed\n" +
			"through untouched (`--param metadata[order]=A17`). Every parameter Stripe\n" +
			"documents for an endpoint is therefore reachable without a dedicated\n" +
			"flag.\n" +
			"\n" +
			"Lists are cursor-paginated, never numbered: --limit is 1-100 and Stripe\n" +
			"defaults to 10 when it is omitted, and paging forward means passing the\n" +
			"last item's id to --starting-after while `has_more` stays true. The search\n" +
			"commands are the exception — they page with --page from the previous\n" +
			"response's `next_page`, and their index lags writes by up to a minute.\n" +
			"\n" +
			"--idempotency-key on any create or refund makes a retry safe; without one,\n" +
			"a repeated call issues a second refund or a second customer. Real money\n" +
			"moves, and a refund cannot be reversed.\n" +
			"\n" +
			"Responses are Stripe's JSON verbatim under a pinned API version, so shapes\n" +
			"do not drift. `get <path>` is the raw read passthrough for any endpoint\n" +
			"without a dedicated verb; there is no write equivalent.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (Stripe responses are always JSON; accepted for uniformity)")

	root.AddCommand(
		s.newBalanceCmd(token),
		s.newListGetGroup(token, "charge", "Inspect payments (charges)", "/charges", longChargeList, longChargeGet),
		s.newListGetGroup(token, "payment-intent", "Inspect payment intents (read-only)", "/payment_intents", longPaymentIntentList, longPaymentIntentGet),
		s.newCustomerCmd(token),
		s.newInvoiceCmd(token),
		s.newSubscriptionCmd(token),
		s.newRefundCmd(token),
		s.newListGetGroup(token, "payout", "Settlement reporting (payouts)", "/payouts", longPayoutList, longPayoutGet),
		s.newListGetGroup(token, "product", "Catalog lookups (products)", "/products", longProductList, longProductGet),
		s.newListGetGroup(token, "price", "Catalog lookups (prices)", "/prices", longPriceList, longPriceGet),
		s.newListGetGroup(token, "dispute", "Chargeback triage (disputes)", "/disputes", longDisputeList, longDisputeGet),
		s.newListGetGroup(token, "event", "Audit trail (events)", "/events", longEventList, longEventGet),
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
