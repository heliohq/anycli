package recurly

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// listFlags holds the query filters shared by every `list` leaf. They map
// directly to Recurly's collection query parameters.
type listFlags struct {
	limit     int
	cursor    string
	state     string
	typ       string
	order     string
	sort      string
	beginTime string
	endTime   string
	account   string // account-scoped lists only
}

// registerListFlags attaches the shared list filters. accountScoped adds
// --account, which reroutes the request under /accounts/<code>/… .
func registerListFlags(cmd *cobra.Command, accountScoped bool) *listFlags {
	lf := &listFlags{}
	f := cmd.Flags()
	f.IntVar(&lf.limit, "limit", 0, "max records to return (1-200)")
	f.StringVar(&lf.cursor, "cursor", "", "pagination cursor from a prior response's next")
	f.StringVar(&lf.state, "state", "", "filter by state")
	f.StringVar(&lf.typ, "type", "", "filter by type")
	f.StringVar(&lf.order, "order", "", "sort order: asc or desc")
	f.StringVar(&lf.sort, "sort", "", "field to sort by (created_at or updated_at)")
	f.StringVar(&lf.beginTime, "begin-time", "", "inclusive lower time bound (ISO-8601)")
	f.StringVar(&lf.endTime, "end-time", "", "exclusive upper time bound (ISO-8601)")
	if accountScoped {
		f.StringVar(&lf.account, "account", "", "scope the list to an account (ID or code-<code>)")
	}
	return lf
}

// query builds the Recurly query string from the set filters (unset fields are
// omitted).
func (lf *listFlags) query() url.Values {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	if lf.limit > 0 {
		q.Set("limit", strconv.Itoa(lf.limit))
	}
	set("cursor", lf.cursor)
	set("state", lf.state)
	set("type", lf.typ)
	set("order", lf.order)
	set("sort", lf.sort)
	set("begin_time", lf.beginTime)
	set("end_time", lf.endTime)
	return q
}

// runList issues a collection GET and emits the provider-neutral list envelope.
func (s *Service) runList(ctx context.Context, key, region, path string, q url.Values) error {
	body, err := s.call(ctx, key, region, "GET", path, q, nil)
	if err != nil {
		return err
	}
	env, err := toListEnvelope(body)
	if err != nil {
		return err
	}
	return s.emitJSON(env)
}

// runGet issues a single-resource request and passes the resource JSON through
// unwrapped.
func (s *Service) runGet(ctx context.Context, key, region, method, path string, q url.Values, payload []byte) error {
	body, err := s.call(ctx, key, region, method, path, q, payload)
	if err != nil {
		return err
	}
	return s.emitJSON(body)
}

// newAccountGroup builds `recurly account …`.
func (s *Service) newAccountGroup(key, region string) *cobra.Command {
	g := newGroupCmd("account", "Look up accounts (customers)")

	list := &cobra.Command{Use: "list", Short: "List accounts", Long: "Accounts are Recurly's customers. Each carries a `code`, the business key\n" +
		"the rest of the tool addresses it by as `code-<account_code>`, so this is\n" +
		"where to resolve a customer before scoping any other list. `--state` narrows\n" +
		"to `active` or `inactive`. Rows carry identity and address, not billing\n" +
		"state — `account balance` and `subscription list --account` answer that.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/accounts", lf.query())
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one account", longAccountGet, func(id string) string { return "/accounts/" + id }),
		s.newGetLeaf(key, region, "balance", "Get an account balance", longAccountBalance, func(id string) string { return "/accounts/" + id + "/balance" }),
		s.newGetLeaf(key, region, "billing-info", "Get an account's billing info", longAccountBillingInfo, func(id string) string { return "/accounts/" + id + "/billing_info" }),
	)
	return g
}

// newSubscriptionGroup builds `recurly subscription …`.
func (s *Service) newSubscriptionGroup(key, region string) *cobra.Command {
	g := newGroupCmd("subscription", "Manage subscriptions")

	list := &cobra.Command{Use: "list", Short: "List subscriptions", Long: "Unscoped this walks every subscription on the site, which is rarely what is\n" +
		"wanted: pass `--account code-<account_code>` to answer \"what is this\n" +
		"customer paying for\" in one page. `--state` accepts `active`, `canceled`,\n" +
		"`expired`, `future` and `in_trial`, and note that `canceled` still means\n" +
		"live until the period ends — `expired` is the one that has actually stopped.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/subscriptions"), lf.query())
	}

	create := &cobra.Command{Use: "create", Short: "Create a subscription", Long: "`--body` is a raw Recurly subscription JSON payload, validated locally as\n" +
		"JSON before anything is sent, and must at minimum name the plan, the\n" +
		"currency and the account:\n" +
		"`{\"plan_code\":\"gold\",\"currency\":\"USD\",\"account\":{\"code\":\"bob\"}}`. An\n" +
		"account referenced by code that does not exist yet is created inline by\n" +
		"Recurly. This bills the customer on the spot unless the body sets a trial or\n" +
		"a future `starts_at`, so it is the most consequential write in the tool.", Args: cobra.NoArgs, Annotations: sideEffect(true)}
	createBody := create.Flags().String("body", "", "subscription JSON body (required)")
	create.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := requireBody(*createBody)
		if err != nil {
			return err
		}
		return s.runGet(cmd.Context(), key, region, "POST", "/subscriptions", nil, payload)
	}

	// Plan upgrades/downgrades and quantity/price/add-on edits go through
	// Create Subscription Change (POST /subscriptions/{id}/change), not the PUT
	// update endpoint (which only edits non-plan fields and rejects plan_code).
	change := s.newBodyLeaf(key, region, "change", "Change a subscription's plan/quantity/price/add-ons", longSubscriptionChange, "POST",
		func(id string) string { return "/subscriptions/" + id + "/change" })

	pause := &cobra.Command{Use: "pause <id>", Short: "Pause a subscription", Long: "`--cycles` is the number of FUTURE billing cycles to skip and is always\n" +
		"sent, so omitting it pauses for zero cycles — which cancels a pending pause\n" +
		"rather than starting one. The pause begins at the end of the current period,\n" +
		"not immediately: the customer keeps access and the current period still\n" +
		"bills. `subscription resume` ends it early.", Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	cycles := pause.Flags().Int("cycles", 0, "number of billing cycles to pause (remaining_pause_cycles)")
	pause.RunE = func(cmd *cobra.Command, args []string) error {
		payload, _ := json.Marshal(map[string]any{"remaining_pause_cycles": *cycles})
		return s.runGet(cmd.Context(), key, region, "PUT", "/subscriptions/"+args[0]+"/pause", nil, payload)
	}

	terminate := &cobra.Command{Use: "terminate <id>", Short: "Terminate a subscription", Long: "Ends the subscription IMMEDIATELY, unlike `subscription cancel`, which lets\n" +
		"the paid period run out. `--refund` picks what happens to money already\n" +
		"taken: `none` keeps it, `partial` refunds the unused remainder of the\n" +
		"period, `full` refunds the whole last charge. Omitting the flag leaves the\n" +
		"choice to the site's own default. There is no undo — reinstating means a new\n" +
		"`subscription create`.", Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	refund := terminate.Flags().String("refund", "", "refund type: none, partial, or full")
	terminate.RunE = func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if *refund != "" {
			q.Set("refund", *refund)
		}
		return s.runGet(cmd.Context(), key, region, "PUT", "/subscriptions/"+args[0]+"/terminate", q, nil)
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one subscription", longSubscriptionGet, func(id string) string { return "/subscriptions/" + id }),
		create,
		change,
		s.newActionLeaf(key, region, "cancel", "Cancel a subscription", longSubscriptionCancel, func(id string) string { return "/subscriptions/" + id + "/cancel" }),
		pause,
		s.newActionLeaf(key, region, "resume", "Resume a paused subscription", longSubscriptionResume, func(id string) string { return "/subscriptions/" + id + "/resume" }),
		terminate,
	)
	return g
}

// newInvoiceGroup builds `recurly invoice …`.
func (s *Service) newInvoiceGroup(key, region string) *cobra.Command {
	g := newGroupCmd("invoice", "Look up invoices and retry collection")

	list := &cobra.Command{Use: "list", Short: "List invoices", Long: "Scope with `--account code-<account_code>` when investigating one customer.\n" +
		"`--state` accepts `pending`, `processing`, `past_due`, `paid`, `failed` and\n" +
		"`voided`; `past_due` plus `failed` is the dunning queue. `--type` separates\n" +
		"`charge` invoices from `credit` ones, which otherwise interleave and make\n" +
		"totals look wrong. Each row's `number` is usable elsewhere as\n" +
		"`number-<invoice_number>`.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/invoices"), lf.query())
	}

	lineItems := &cobra.Command{Use: "line-items <id>", Short: "List an invoice's line items", Long: "The per-charge breakdown behind one invoice: each item carries `description`,\n" +
		"`amount`, `quantity`, its tax, and whether it is a `charge` or a `credit`.\n" +
		"This is what explains a total the customer disputes. It takes no filter or\n" +
		"paging flags — the whole invoice is returned in the list envelope.", Args: cobra.ExactArgs(1), Annotations: sideEffect(false)}
	lineItems.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runList(cmd.Context(), key, region, "/invoices/"+args[0]+"/line_items", nil)
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one invoice", longInvoiceGet, func(id string) string { return "/invoices/" + id }),
		lineItems,
		s.newActionLeaf(key, region, "collect", "Retry collection on an invoice", longInvoiceCollect, func(id string) string { return "/invoices/" + id + "/collect" }),
	)
	return g
}

// newTransactionGroup builds `recurly transaction …`.
func (s *Service) newTransactionGroup(key, region string) *cobra.Command {
	g := newGroupCmd("transaction", "Look up payment transactions")

	list := &cobra.Command{Use: "list", Short: "List transactions", Long: "A transaction is one payment ATTEMPT, so a single invoice can have several\n" +
		"here — declines included. Scope with `--account code-<account_code>` and\n" +
		"narrow with `--type` (`purchase`, `refund`, `verify`) or `--state`\n" +
		"(`success`, `failed`, `void`). This is the read that shows whether a\n" +
		"customer's card is failing repeatedly rather than once.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/transactions"), lf.query())
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one transaction", longTransactionGet, func(id string) string { return "/transactions/" + id }),
	)
	return g
}

// newPlanGroup builds `recurly plan …`.
func (s *Service) newPlanGroup(key, region string) *cobra.Command {
	g := newGroupCmd("plan", "Look up plans (catalog)")
	list := &cobra.Command{Use: "list", Short: "List plans", Long: "The site's product catalog: the `code` on each plan is what a\n" +
		"`subscription create` body puts in `plan_code`, and it is not the plan name.\n" +
		"`--state` separates `active` plans from `inactive` ones still attached to\n" +
		"existing subscriptions. Each plan's `currencies[]` fixes which currencies a\n" +
		"subscription against it may be created in.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/plans", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one plan", longPlanGet, func(id string) string { return "/plans/" + id }))
	return g
}

// newCouponGroup builds `recurly coupon …`.
func (s *Service) newCouponGroup(key, region string) *cobra.Command {
	g := newGroupCmd("coupon", "Look up coupons (discounts)")
	list := &cobra.Command{Use: "list", Short: "List coupons", Long: "`--state` narrows to `redeemable`, `expired` or `maxed_out`. A coupon's\n" +
		"`code` is what a redemption references, and `discount` carries either a\n" +
		"percent or a fixed amount per currency. This tool can read coupons but\n" +
		"cannot create one or redeem it against an account.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/coupons", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one coupon", longCouponGet, func(id string) string { return "/coupons/" + id }))
	return g
}

// newLineItemGroup builds `recurly line-item …`.
func (s *Service) newLineItemGroup(key, region string) *cobra.Command {
	g := newGroupCmd("line-item", "Look up line items")
	list := &cobra.Command{Use: "list", Short: "List line items", Long: "The site-wide feed of individual charges and credits, across invoices and\n" +
		"not yet invoiced. Scope it with `--account code-<account_code>`; `--state`\n" +
		"separates `pending` items (awaiting the next invoice) from `invoiced` ones.\n" +
		"To read the lines of ONE known invoice, `invoice line-items <id>` is the\n" +
		"direct path and takes no filters.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/line_items"), lf.query())
	}
	g.AddCommand(list)
	return g
}

// newSiteGroup builds `recurly site …`.
func (s *Service) newSiteGroup(key, region string) *cobra.Command {
	g := newGroupCmd("site", "Look up Recurly sites")
	list := &cobra.Command{Use: "list", Short: "List sites", Long: "A Recurly site is one environment — typically a production and a sandbox\n" +
		"site per business. The API key fixes which sites are reachable, so this\n" +
		"usually returns one row, and its `subdomain` is how to confirm which\n" +
		"environment the connection is actually pointed at before making a write.", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/sites", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one site", longSiteGet, func(id string) string { return "/sites/" + id }))
	return g
}

// sideEffect builds the design-318 side-effect annotation map for a runnable
// leaf (write = "true", read = "false").
func sideEffect(write bool) map[string]string {
	if write {
		return map[string]string{"anycli.side_effect": "true"}
	}
	return map[string]string{"anycli.side_effect": "false"}
}

// The Longs below are declared next to the shared leaf builders because it is
// the builder — not the call site — that fixes the single positional id
// argument and the request shape each of these commands describes.
const (
	longAccountGet = "Accepts the opaque id or the far more useful `code-<account_code>` alias, so\n" +
		"a customer known by business code needs no prior lookup. Returns identity,\n" +
		"address and `state` (`active` or `inactive`), but no money: use\n" +
		"`account balance` for what is owed and `account billing-info` for the card."

	longAccountBalance = "The fastest answer to \"does this customer owe anything\": a `past_due`\n" +
		"boolean plus per-currency `balances[]`, without paging invoices. It says\n" +
		"nothing about WHICH invoice is unpaid — follow with\n" +
		"`invoice list --account code-<code> --state past_due` for that."

	longAccountBillingInfo = "The stored payment method: card brand, last four digits, expiry, and the\n" +
		"billing address Recurly will charge against. Check it first when an invoice\n" +
		"keeps failing, because `invoice collect` against an expired or removed card\n" +
		"simply fails again. An account that has never had billing info stored has\n" +
		"no record here at all."

	longSubscriptionGet = "Returns `state` (`active`, `canceled`, `expired`, `paused`, `future`),\n" +
		"`current_period_started_at` / `current_period_ends_at`, the plan snapshot\n" +
		"and `unit_amount`. Read `state` carefully: `canceled` means it will not\n" +
		"renew but is still live and still grants access until the period ends —\n" +
		"`expired` is the one that has actually stopped. Accepts `uuid-<uuid>`."

	longSubscriptionChange = "Goes through Recurly's Create Subscription Change endpoint, not the plain\n" +
		"update, which is why this — and not some update verb — is the path for plan\n" +
		"upgrades and downgrades, quantity, unit price and add-on edits. `--body` is\n" +
		"raw Recurly JSON (`{\"plan_code\":\"silver\"}`) and is rejected locally if it is\n" +
		"not valid JSON. A change is scheduled for the next renewal unless the body\n" +
		"sets `\"timeframe\":\"now\"`, which applies it immediately with proration."

	longSubscriptionCancel = "Cancels at the END of the paid period: the customer keeps access until\n" +
		"`current_period_ends_at`, nothing is refunded, and the state becomes\n" +
		"`canceled` rather than `expired`. `subscription resume` undoes it while the\n" +
		"period is still running. To stop access now instead, use\n" +
		"`subscription terminate`."

	longSubscriptionResume = "Clears both a pause and a pending cancellation, putting the subscription\n" +
		"back to `active` on its original billing schedule. It cannot revive an\n" +
		"`expired` subscription — once the period has actually elapsed the only path\n" +
		"is a fresh `subscription create`, which starts a new billing relationship."

	longInvoiceGet = "Accepts `number-<invoice_number>`, which is the number a customer quotes\n" +
		"from their receipt. Returns `state`, `total` and `balance` — the amount\n" +
		"still owed, which is what distinguishes a partially paid invoice from an\n" +
		"unpaid one. The charge breakdown is NOT included; `invoice line-items` has\n" +
		"it, and `transaction list` has the payment attempts."

	longInvoiceCollect = "Charges the account's stored billing info again, right now, outside\n" +
		"Recurly's own dunning schedule — so calling it repeatedly means repeated\n" +
		"real charge attempts against a real card, and each failure can count\n" +
		"against the gateway's retry reputation. Only an invoice with an outstanding\n" +
		"balance is collectable. Verify the card with `account billing-info` first,\n" +
		"since a retry against an expired card fails identically."

	longTransactionGet = "One payment ATTEMPT, not an invoice. `status` is `success`, `declined`,\n" +
		"`error` or `void`, and `status_code` / `status_message` carry the gateway's\n" +
		"own reason — this is the only place the actual cause of a decline appears,\n" +
		"since the invoice records only that collection failed."

	longPlanGet = "Accepts `code-<plan_code>` as well as the opaque id. Returns the pricing a\n" +
		"new subscription would inherit: `currencies[]` with the unit amount per\n" +
		"currency, the billing `interval_unit` / `interval_length`, and any trial\n" +
		"settings. Existing subscriptions keep the price they were created at, so\n" +
		"this is the catalog price, not what any given customer pays."

	longCouponGet = "Accepts `code-<coupon_code>`. Returns the discount as either a percentage\n" +
		"or a fixed amount per currency, plus `state` (`redeemable`, `expired`,\n" +
		"`maxed_out`), the redemption limits and which plans it applies to. Reading\n" +
		"is all this tool does — redeeming a coupon against an account is not\n" +
		"exposed."

	longSiteGet = "Returns one site's `subdomain`, `mode` (`production` or `sandbox`) and\n" +
		"`state`. Worth calling once before any write to confirm the key really\n" +
		"points at the environment intended, because nothing else in the output of\n" +
		"the other commands says whether this is production or a sandbox."
)

// newGetLeaf builds a single-resource GET (read) leaf taking one positional id.
func (s *Service) newGetLeaf(key, region, use, short, long string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Long: long, Args: cobra.ExactArgs(1), Annotations: sideEffect(false)}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runGet(cmd.Context(), key, region, "GET", path(args[0]), nil, nil)
	}
	return c
}

// newActionLeaf builds a bodyless PUT action (write) leaf taking one positional
// id (cancel/resume/collect).
func (s *Service) newActionLeaf(key, region, use, short, long string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Long: long, Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runGet(cmd.Context(), key, region, "PUT", path(args[0]), nil, nil)
	}
	return c
}

// newBodyLeaf builds a write leaf taking one positional id plus a required
// --body JSON payload (subscription change).
func (s *Service) newBodyLeaf(key, region, use, short, long, method string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Long: long, Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	body := c.Flags().String("body", "", "JSON body (required)")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		payload, err := requireBody(*body)
		if err != nil {
			return err
		}
		return s.runGet(cmd.Context(), key, region, method, path(args[0]), nil, payload)
	}
	return c
}

// accountScoped returns the account-scoped collection path when account is set,
// otherwise the top-level collection path. The tail is derived from the
// top-level path (e.g. "/subscriptions" → "/accounts/<code>/subscriptions").
func accountScoped(account, topLevel string) string {
	if account == "" {
		return topLevel
	}
	return "/accounts/" + account + topLevel
}

// requireBody validates a --body flag value: it must be present and valid JSON.
// Both failures are usage errors (exit 2) that must not reach the network.
func requireBody(raw string) ([]byte, error) {
	if raw == "" {
		return nil, &usageError{msg: "--body is required (JSON payload)"}
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, &usageError{msg: "--body is not valid JSON: " + err.Error()}
	}
	return []byte(raw), nil
}
