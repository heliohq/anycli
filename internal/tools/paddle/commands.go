package paddle

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sideEffectAnnotation is the design-318 side-effect key Inspect reads to decide
// whether a leaf needs approval. Kept as a literal (the root-package constant is
// unexported and importing it would cycle).
const sideEffectAnnotation = "anycli.side_effect"

func sideEffect(mutates bool) map[string]string {
	return map[string]string{sideEffectAnnotation: strconv.FormatBool(mutates)}
}

// newRoot builds the grouped-by-resource cobra tree. Each resource is a runnable
// group; verbs hang under it.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "paddle",
		Short: "Paddle Billing built-in service (subscription billing, merchant of record)",
		Long: "This is Paddle Billing, the current product, not the legacy Paddle Classic\n" +
			"vendor API. Paddle is the merchant of record, so everything here is the\n" +
			"seller's own customers, subscriptions and revenue.\n" +
			"\n" +
			"The API key's prefix picks the environment: a live key reaches the live\n" +
			"account and a sandbox key the sandbox one. They are separate accounts\n" +
			"holding separate data — a sandbox key never sees a live customer — and\n" +
			"there is no environment flag to set.\n" +
			"\n" +
			"Default output prints just the resource `data`. The global `--json` adds\n" +
			"the meta envelope, which is where pagination lives: read\n" +
			"`meta.pagination.next` and hand it back as `--after <cursor>`, sizing\n" +
			"pages with `--per-page`. Lists also take `--status` plus a repeatable\n" +
			"`--filter key=value` for any query parameter without a typed flag.\n" +
			"\n" +
			"Every create, update and lifecycle action carries its request body as raw\n" +
			"JSON in `--data` — required on create, update and the previews, optional\n" +
			"on the subscription actions.\n" +
			"\n" +
			"Money-moving actions have dry-run twins: `subscription preview-update`\n" +
			"before `subscription update`, `subscription preview-charge` before\n" +
			"`subscription charge`, `transaction preview` before `transaction create`.\n" +
			"Run the preview, read the proration and total it computes, then commit.\n" +
			"\n" +
			"Nothing here has a delete verb, because financial records are never\n" +
			"deleted. Cancel, pause, and moving an entity to an archived status are the\n" +
			"entire mutation surface, and money is given back with `adjustment create`\n" +
			"rather than by undoing a transaction.\n" +
			"\n" +
			"Failures print Paddle's error object with its `code`, `detail` and\n" +
			"`documentation_url`. A 401 or 403 means the key itself has to be replaced;\n" +
			"a 429 is Paddle's rate limit (roughly 240 requests a minute) and calls for\n" +
			"a back-off, not a retry loop.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured {data, meta} JSON output")

	root.AddCommand(
		s.customerGroup(token),
		s.subscriptionGroup(token),
		s.transactionGroup(token),
		s.catalogGroup(token, "product", "/products", "Manage products", productLongs),
		s.catalogGroup(token, "price", "/prices", "Manage prices", priceLongs),
		s.catalogGroup(token, "discount", "/discounts", "Manage discounts", discountLongs),
		s.adjustmentGroup(token),
		s.reportGroup(token),
		s.eventGroup(token),
	)
	return root
}

// The customer group's Long texts. They live beside the group constructor
// because it is the call site there, not the generic list/get/create builder,
// that binds each text to the /customers endpoint it describes.
const (
	longCustomerList = "Paddle customers are billing records keyed by email, not application\n" +
		"users. `--status` takes `active` or `archived`. There is no name or\n" +
		"email search flag, so find one with `--filter email=<address>`, which\n" +
		"maps straight onto Paddle's own query parameter. Page with `--after`\n" +
		"taken from `meta.pagination.next`, which only appears under `--json`."
	longCustomerGet = "Takes a `ctm_…` customer id. Returns the billing record — email, name,\n" +
		"locale, status — and nothing about what the customer bought:\n" +
		"subscriptions come from `subscription list --customer-id` and payments\n" +
		"from `transaction list --customer-id`."
	longCustomerCreate = "--data is required and needs at least an `email`, which is the\n" +
		"customer's identity to Paddle. Look first with `customer list --filter\n" +
		"email=<address>`: creating a second customer on an address already in\n" +
		"use is rejected rather than merged."
	longCustomerUpdate = "Takes a `ctm_…` id and a required --data body carrying only the fields\n" +
		"to change. It is also the retirement path — there is no delete verb\n" +
		"anywhere in this tool, so setting `status` to `archived` is how a\n" +
		"customer is taken out of use."
	longCustomerCreditBalances = "Takes a `ctm_…` id and returns the credit the customer holds per\n" +
		"currency, split into available, reserved and used. Credit is produced by\n" +
		"an `adjustment create` of type credit; there is no command here that\n" +
		"spends it."
	longCustomerAddresses = "Takes the `ctm_…` CUSTOMER id, not an address id. Addresses matter\n" +
		"beyond bookkeeping: Paddle derives sales tax and VAT from the billing\n" +
		"country, so what is on file here determines what the customer is\n" +
		"actually charged."
	longCustomerBusinesses = "Takes a `ctm_…` id. A business is the B2B layer on a customer — company\n" +
		"name and tax identifier — used for invoiced rather than card billing. A\n" +
		"customer with no business attached is a plain B2C billing record."
)

func (s *Service) customerGroup(token string) *cobra.Command {
	g := newGroupCmd("customer", "Look up and manage customers")
	g.AddCommand(
		s.listCmd(token, "/customers", longCustomerList, nil),
		s.getCmd(token, "/customers", "get", "Get one customer", longCustomerGet),
		s.createCmd(token, "/customers", "Create a customer", longCustomerCreate),
		s.updateCmd(token, "/customers", "Update a customer", longCustomerUpdate),
		s.subGetCmd(token, "/customers", "credit-balances", "Get a customer's credit balances", longCustomerCreditBalances),
		s.subGetCmd(token, "/customers", "addresses", "List a customer's addresses", longCustomerAddresses),
		s.subGetCmd(token, "/customers", "businesses", "List a customer's businesses", longCustomerBusinesses),
	)
	return g
}

// The subscription group's Long texts.
const (
	longSubscriptionList = "`--customer-id ctm_…` is the typed filter; `--status` takes Paddle's\n" +
		"subscription states (`active`, `trialing`, `paused`, `canceled`,\n" +
		"`past_due`). Canceled subscriptions are returned unless the status\n" +
		"filter excludes them, so \"is this customer paying us\" has to read the\n" +
		"status rather than count rows."
	longSubscriptionGet = "Takes a `sub_…` id. The load-bearing fields are `status`,\n" +
		"`next_billed_at`, `items` (the price each seat or add-on sits on) and\n" +
		"`scheduled_change`, which is non-null when a pause or cancellation is\n" +
		"already booked for a future date — a subscription can read `active`\n" +
		"while a cancellation is pending."
	longSubscriptionUpdate = "Takes a `sub_…` id and a required --data body. This is the plan-change\n" +
		"path: replacing `items` moves the customer onto a different price, and\n" +
		"`proration_billing_mode` decides whether the remainder of the current\n" +
		"period is charged or credited immediately, deferred to the next bill, or\n" +
		"ignored. Run `subscription preview-update` with the same body first —\n" +
		"under the immediate proration modes money moves the moment this returns."
	longSubscriptionCancel = "Takes a `sub_…` id. With no --data Paddle cancels at the end of the\n" +
		"current billing period; an `effective_from` of `immediately` in --data\n" +
		"ends it at once. An immediate cancel does not refund the unused period\n" +
		"by itself — that is a separate `adjustment create`. Cancellation is\n" +
		"terminal: a canceled subscription cannot be resumed, only replaced by a\n" +
		"new one."
	longSubscriptionPause = "Takes a `sub_…` id. Pausing stops billing while keeping the subscription\n" +
		"and its items intact, so `subscription resume` can pick it up later —\n" +
		"that is the difference from `subscription cancel`, which is terminal.\n" +
		"With no --data the pause starts at the end of the current billing\n" +
		"period; an `effective_from` of `immediately` pauses it now, and a\n" +
		"`resume_at` schedules the restart in advance."
	longSubscriptionResume = "Takes a `sub_…` id and applies to a paused subscription. With no --data\n" +
		"it resumes immediately; an `effective_from` in --data schedules it.\n" +
		"Billing restarts on the subscription's existing items and prices, so\n" +
		"changing what the customer pays is a separate `subscription update`."
	longSubscriptionActivate = "Takes a `sub_…` id and applies only to a subscription in `trialing`: it\n" +
		"ends the trial early and bills the customer NOW instead of at the\n" +
		"trial's scheduled end. It is not a way to un-pause — that is\n" +
		"`subscription resume` — and it cannot revive a canceled subscription."
	longSubscriptionCharge = "Takes a `sub_…` id and bills a one-off amount against the subscription's\n" +
		"stored payment method, outside the normal cycle: an add-on, an overage,\n" +
		"a setup fee. --data carries the items to charge and `effective_from`.\n" +
		"The money moves immediately, so run `subscription preview-charge` with\n" +
		"the same body and read the total it returns first."
	longSubscriptionPreviewCharge = "The dry run for `subscription charge`: the same `sub_…` id and the same\n" +
		"--data body, but nothing is billed. It returns the amounts, tax and\n" +
		"totals the real charge would produce, which is the figure to put in\n" +
		"front of a user before committing. It writes nothing and is safe to\n" +
		"repeat."
	longSubscriptionPreviewUpdate = "The dry run for `subscription update`: the same `sub_…` id and the same\n" +
		"--data body, with no change applied. It returns the proration Paddle\n" +
		"would compute — the immediate charge or credit, and the next billing\n" +
		"amount and date — which is what answers \"what will this plan change cost\n" +
		"them\". Paddle serves it as a PATCH, mirroring the real update verb."
)

func (s *Service) subscriptionGroup(token string) *cobra.Command {
	g := newGroupCmd("subscription", "Look up and manage subscriptions")
	g.AddCommand(
		s.listCmd(token, "/subscriptions", longSubscriptionList, func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
		}),
		s.getCmd(token, "/subscriptions", "get", "Get one subscription", longSubscriptionGet),
		s.updateCmd(token, "/subscriptions", "Update a subscription (items, proration)", longSubscriptionUpdate),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "cancel", "cancel", "Cancel a subscription", longSubscriptionCancel, true),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "pause", "pause", "Pause a subscription", longSubscriptionPause, true),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "resume", "resume", "Resume a subscription", longSubscriptionResume, true),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "activate", "activate", "Activate a trialing subscription", longSubscriptionActivate, true),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "charge", "charge", "Create a one-time charge on a subscription", longSubscriptionCharge, true),
		s.actionCmd(token, http.MethodPost, "/subscriptions", "preview-charge", "charge/preview", "Preview a one-time charge (dry run)", longSubscriptionPreviewCharge, false),
		// Preview-update mirrors the real update verb: Paddle serves it as
		// PATCH /subscriptions/{id}/preview (not POST).
		s.actionCmd(token, http.MethodPatch, "/subscriptions", "preview-update", "preview", "Preview a subscription update (dry run)", longSubscriptionPreviewUpdate, false),
	)
	return g
}

// The transaction group's Long texts.
const (
	longTransactionList = "Transactions are Paddle's billing records — a payment, an invoice or a\n" +
		"billable draft — and are what a \"did their payment go through\" question\n" +
		"actually reads. `--customer-id ctm_…` and `--subscription-id sub_…` are\n" +
		"the typed filters; `--status` takes `draft`, `ready`, `billed`, `paid`,\n" +
		"`completed`, `canceled` or `past_due`. A failed payment appears as a\n" +
		"`past_due` row, not as a missing one."
	longTransactionGet = "Takes a `txn_…` id and returns the line items, the tax breakdown, the\n" +
		"payment attempts and the totals. When the question is why a charge\n" +
		"failed, read `payments`: the transaction status alone does not say which\n" +
		"attempt was declined or why."
	longTransactionCreate = "--data is required. This bills a customer or raises an invoice outside\n" +
		"any subscription — a one-off sale, a manual invoice. Real money moves\n" +
		"once the transaction leaves draft, so run `transaction preview` with the\n" +
		"same body first. There is no delete verb: a transaction is undone by\n" +
		"cancelling it or offset with `adjustment create`."
	longTransactionInvoice = "Takes a `txn_…` id and returns a URL to the invoice PDF, not the PDF\n" +
		"bytes. Paddle signs the URL and expires it quickly, so fetch or hand it\n" +
		"over promptly rather than storing it. Only a billed or completed\n" +
		"transaction has an invoice to point at."
	longTransactionPreview = "Takes no id: it prices a hypothetical transaction from the --data body,\n" +
		"which makes it the way to quote a cart or check what tax a given\n" +
		"customer will attract before `transaction create`. Include a\n" +
		"`customer_id`, or an address with its country, or the tax figure it\n" +
		"returns is a default rather than that customer's real one."
)

func (s *Service) transactionGroup(token string) *cobra.Command {
	g := newGroupCmd("transaction", "Look up transactions and invoices")
	g.AddCommand(
		s.listCmd(token, "/transactions", longTransactionList, func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
			c.Flags().String("subscription-id", "", "filter by subscription id (sub_…)")
		}),
		s.getCmd(token, "/transactions", "get", "Get one transaction", longTransactionGet),
		s.createCmd(token, "/transactions", "Create a transaction", longTransactionCreate),
		s.subGetCmd(token, "/transactions", "invoice", "Get a transaction's invoice PDF URL", longTransactionInvoice),
		s.previewCmd(token, "/transactions/preview", "preview", "Preview a transaction (dry run)", longTransactionPreview),
	)
	return g
}

// products, prices and discounts share one builder but not their prose: the
// same verb means a different thing on each, so every catalog resource brings
// its own four texts.
const (
	longProductList = "A product is the catalog entity a price hangs off, and a product with no\n" +
		"price cannot be sold. `--status` takes `active` or `archived`. There is\n" +
		"no name-search flag, so anything else goes through `--filter key=value`."
	longProductGet = "Takes a `pro_…` product id. Returns the name, tax category and custom\n" +
		"data, but NOT the prices — those come from `price list --filter\n" +
		"product_id=pro_…`."
	longProductCreate = "--data is required and needs a `name` and a `tax_category`. The tax\n" +
		"category is the consequential field: it drives how Paddle charges VAT\n" +
		"and sales tax on everything priced from this product. A new product is\n" +
		"not sellable until a price is created against it."
	longProductUpdate = "Takes a `pro_…` id and a --data body of just the fields to change. There\n" +
		"is no delete verb, so setting `status` to `archived` is how a product is\n" +
		"retired."
)

// The price group's Long texts.
const (
	longPriceList = "Prices carry the actual money — amount, currency and billing cycle — and\n" +
		"each belongs to exactly one product. `--filter product_id=pro_…` narrows\n" +
		"to one product's prices and `--status` takes `active` or `archived`. A\n" +
		"subscription's `items` reference price ids, so this is where the target\n" +
		"of a plan change comes from."
	longPriceGet = "Takes a `pri_…` price id and returns the unit amount per currency, the\n" +
		"billing interval, any trial period and the quantity limits. The amount\n" +
		"is a STRING in the currency's smallest unit: 1000 with a currency_code\n" +
		"of USD is $10.00, not $1000."
	longPriceCreate = "--data is required and must reference an existing `product_id` plus a\n" +
		"`unit_price` whose `amount` is a string in the currency's smallest unit\n" +
		"— 1000 in USD is $10.00. Including a `billing_cycle` makes the price\n" +
		"recurring; omitting it makes it a one-time price."
	longPriceUpdate = "Takes a `pri_…` id and a --data body of just the fields to change. There\n" +
		"is no delete verb, so retiring a price means setting `status` to\n" +
		"`archived`."
)

// The discount group's Long texts.
const (
	longDiscountList = "Discounts are account-level coupon definitions — a percentage or a flat\n" +
		"amount — that transactions and subscriptions reference by id. `--status`\n" +
		"takes `active`, `archived` or `expired`. Find one by the code customers\n" +
		"type with `--filter code=<code>`."
	longDiscountGet = "Takes a `dsc_…` discount id. Returns the type and amount, the code, the\n" +
		"expiry, the usage limit and the redemption count — `times_used` against\n" +
		"`usage_limit` is what answers whether a campaign code is exhausted."
	longDiscountCreate = "--data is required: a `type` of percentage or flat, an `amount` as a\n" +
		"string (for a flat discount, in the currency's smallest unit) and a\n" +
		"`description`. A `code` is what a customer types at checkout; with no\n" +
		"code the discount can only be applied by id from the API. `restrict_to`\n" +
		"limits it to named price ids and `usage_limit` caps total redemptions."
	longDiscountUpdate = "Takes a `dsc_…` id and a --data body. Ending a live campaign means\n" +
		"setting `expires_at`, or moving `status` to `archived`; there is no\n" +
		"delete verb."
)

// catalogLongs carries the four Long texts one catalog resource contributes to
// the shared catalogGroup builder.
type catalogLongs struct{ list, get, create, update string }

var (
	productLongs  = catalogLongs{longProductList, longProductGet, longProductCreate, longProductUpdate}
	priceLongs    = catalogLongs{longPriceList, longPriceGet, longPriceCreate, longPriceUpdate}
	discountLongs = catalogLongs{longDiscountList, longDiscountGet, longDiscountCreate, longDiscountUpdate}
)

// catalogGroup builds the list/get/create/update shape shared by products,
// prices, and discounts.
func (s *Service) catalogGroup(token, use, path, short string, longs catalogLongs) *cobra.Command {
	g := newGroupCmd(use, short)
	g.AddCommand(
		s.listCmd(token, path, longs.list, nil),
		s.getCmd(token, path, "get", "Get one "+use, longs.get),
		s.createCmd(token, path, "Create a "+use, longs.create),
		s.updateCmd(token, path, "Update a "+use, longs.update),
	)
	return g
}

// The adjustment group's Long texts.
const (
	longAdjustmentList = "Adjustments are the refund and credit records raised against\n" +
		"transactions — the audit trail for money given back. `--customer-id` and\n" +
		"`--subscription-id` are the typed filters, and `--status` takes\n" +
		"`pending_approval`, `approved` or `rejected`, so a refund appearing here\n" +
		"is not proof it completed."
	longAdjustmentCreate = "This is how money is returned. --data carries a `transaction_id`, an\n" +
		"`action` of `refund`, `credit` or `credit_reverse`, and either a\n" +
		"full-transaction or per-item amount. A refund returns money to the\n" +
		"customer's payment method and cannot be reversed; a credit only offsets\n" +
		"their future invoices and shows up under `customer credit-balances`. The\n" +
		"response can come back `pending_approval` rather than done — read the\n" +
		"status instead of assuming the money moved."
)

func (s *Service) adjustmentGroup(token string) *cobra.Command {
	g := newGroupCmd("adjustment", "List and create refunds/credits")
	g.AddCommand(
		s.listCmd(token, "/adjustments", longAdjustmentList, func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
			c.Flags().String("subscription-id", "", "filter by subscription id (sub_…)")
		}),
		s.createCmd(token, "/adjustments", "Create an adjustment (refund or credit)", longAdjustmentCreate),
	)
	return g
}

// The report group's Long texts.
const (
	longReportCreate = "--data is required and names the report `type` plus its filters,\n" +
		"including the date range. Generation is asynchronous: this returns a\n" +
		"report id in a pending state and the CSV only exists once `report get`\n" +
		"shows it ready. Poll `report get`, then call `report download-url`."
	longReportList = "Returns the reports created on this account with their status. Paddle\n" +
		"expires a generated report after a while, so an id that is still listed\n" +
		"may no longer be downloadable. `--status` filters to `pending`, `ready`,\n" +
		"`failed` or `expired`."
	longReportGet = "Takes a report id and returns its status — the poll target after `report\n" +
		"create`. `pending` means still generating, `ready` means `report\n" +
		"download-url` will work, and `failed` means the request has to be\n" +
		"created again rather than retried on the same id."
	longReportDownloadURL = "Takes a report id and returns a signed CSV URL, not the CSV itself. The\n" +
		"URL is short-lived, so fetch it immediately. It only resolves for a\n" +
		"report whose status is ready — check `report get` first."
)

func (s *Service) reportGroup(token string) *cobra.Command {
	g := newGroupCmd("report", "Create and download revenue reports")
	g.AddCommand(
		s.createCmd(token, "/reports", "Create a report", longReportCreate),
		s.listCmd(token, "/reports", longReportList, nil),
		s.getCmd(token, "/reports", "get", "Get one report", longReportGet),
		s.subGetCmd(token, "/reports", "download-url", "Get a report's CSV download URL", longReportDownloadURL),
	)
	return g
}

// The event group's Long texts.
const (
	longEventList = "The account's event stream: every entity change Paddle recorded, in\n" +
		"occurrence order, which is what reconstructs how a subscription reached\n" +
		"its current state when the current state alone is not enough.\n" +
		"Cursor-paginated like the other lists, so read `meta.pagination.next`\n" +
		"into `--after`. Paddle retains events for a limited window, so this is\n" +
		"not a complete account history."
	longEventTypes = "Lists every event type Paddle can emit and the entity each relates to.\n" +
		"It is the reference both for reading the `event list` stream and for\n" +
		"knowing what a notification destination can subscribe to. Takes no id\n" +
		"and no flags."
	longEventNotificationSettings = "Lists the account's configured webhook and email destinations, the event\n" +
		"types each subscribes to, and whether each is active. This is the read\n" +
		"that answers why a system is not receiving Paddle webhooks. Destinations\n" +
		"are configured in the Paddle dashboard; this tool only reads them."
)

func (s *Service) eventGroup(token string) *cobra.Command {
	g := newGroupCmd("event", "Audit events and notification settings")
	g.AddCommand(
		s.listCmd(token, "/events", longEventList, nil),
		s.rawGetCmd(token, "/event-types", "types", "List available event types", longEventTypes),
		s.rawGetCmd(token, "/notification-settings", "notification-settings", "List notification (webhook) settings", longEventNotificationSettings),
	)
	return g
}

// listCmd is GET <path> with cursor pagination + status/filter query flags.
func (s *Service) listCmd(token, path, long string, extra func(*cobra.Command)) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + strings.TrimPrefix(path, "/"),
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			q, err := listQuery(c)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodGet, path, q, nil)
		},
	}
	cmd.Flags().String("status", "", "filter by status")
	cmd.Flags().String("after", "", "pagination cursor from meta.pagination.next")
	cmd.Flags().Int("per-page", 0, "results per page")
	cmd.Flags().StringArray("filter", nil, "extra filter key=value (repeatable)")
	if extra != nil {
		extra(cmd)
	}
	return cmd
}

// getCmd is GET <path>/<id>.
func (s *Service) getCmd(token, path, use, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, args []string) error {
			return s.run(c, token, http.MethodGet, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// rawGetCmd is GET <path> for account-level collections that take no id.
func (s *Service) rawGetCmd(token, path, use, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			return s.run(c, token, http.MethodGet, path, nil, nil)
		},
	}
}

// subGetCmd is GET <path>/<id>/<sub>.
func (s *Service) subGetCmd(token, path, sub, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         sub + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, args []string) error {
			return s.run(c, token, http.MethodGet, path+"/"+url.PathEscape(args[0])+"/"+sub, nil, nil)
		},
	}
}

// createCmd is POST <path> with a required --data JSON body.
func (s *Service) createCmd(token, path, short, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "create",
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(c *cobra.Command, _ []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPost, path, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// updateCmd is PATCH <path>/<id> with a required --data JSON body.
func (s *Service) updateCmd(token, path, short, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(c *cobra.Command, args []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPatch, path+"/"+url.PathEscape(args[0]), nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// actionCmd is <method> <path>/<id>/<sub> with an optional --data JSON body.
// Most subscription actions are POST; preview-update is PATCH, mirroring the
// real update verb, so the HTTP method is a parameter.
func (s *Service) actionCmd(token, method, path, use, sub, short, long string, mutates bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(mutates),
		RunE: func(c *cobra.Command, args []string) error {
			body, err := dataFlag(c, false)
			if err != nil {
				return err
			}
			return s.run(c, token, method, path+"/"+url.PathEscape(args[0])+"/"+sub, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// previewCmd is POST <path> (no id) with a required --data JSON body — a
// read-only dry run.
func (s *Service) previewCmd(token, path, use, short string, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPost, path, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

func dataFlagDef(cmd *cobra.Command) {
	cmd.Flags().String("data", "", "request body as a JSON object")
}

// dataFlag reads and validates the --data JSON body. When required, an empty
// value is a usage error; when optional, an empty value sends no body.
func dataFlag(cmd *cobra.Command, required bool) ([]byte, error) {
	raw := strings.TrimSpace(mustString(cmd, "data"))
	if raw == "" {
		if required {
			return nil, newUsageError("--data is required (a JSON request body)")
		}
		return nil, nil
	}
	if !json.Valid([]byte(raw)) {
		return nil, newUsageError("--data is not valid JSON")
	}
	return []byte(raw), nil
}

// listQuery builds the list query from the registered flags. status/after/
// per-page map to Paddle's documented query params; --filter carries any other
// key=value pair; convenience --customer-id / --subscription-id map to the
// documented filter params.
func listQuery(cmd *cobra.Command) (url.Values, error) {
	q := url.Values{}
	if v := strings.TrimSpace(mustString(cmd, "status")); v != "" {
		q.Set("status", v)
	}
	if v := strings.TrimSpace(mustString(cmd, "after")); v != "" {
		q.Set("after", v)
	}
	if cmd.Flags().Changed("per-page") {
		n, _ := cmd.Flags().GetInt("per-page")
		if n > 0 {
			q.Set("per_page", strconv.Itoa(n))
		}
	}
	if f := cmd.Flags().Lookup("customer-id"); f != nil {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			q.Set("customer_id", v)
		}
	}
	if f := cmd.Flags().Lookup("subscription-id"); f != nil {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			q.Set("subscription_id", v)
		}
	}
	filters, _ := cmd.Flags().GetStringArray("filter")
	for _, kv := range filters {
		key, value, ok := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, newUsageError("--filter %q must be key=value", kv)
		}
		q.Set(key, strings.TrimSpace(value))
	}
	return q, nil
}

func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// run executes one call and emits the result, honoring the root --json flag.
func (s *Service) run(cmd *cobra.Command, token, method, path string, q url.Values, body []byte) error {
	jsonMode, _ := cmd.Root().PersistentFlags().GetBool("json")
	env, err := s.call(cmd.Context(), token, method, path, q, body)
	if err != nil {
		return err
	}
	return s.emit(jsonMode, env)
}
