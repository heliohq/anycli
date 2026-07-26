package chargebee

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sideEffectFalse / sideEffectTrue are the design-318 annotation maps a leaf
// carries so Inspect/approval-gate coverage can classify it without executing.
func sideEffect(write bool) map[string]string {
	return map[string]string{"anycli.side_effect": strconv.FormatBool(write)}
}

// The per-resource depth texts. They live here, next to the shared builders,
// because one `listCmd` / `getCmdForResource` serves nine resources: the verb
// is generic but what a caller needs to know — which filters matter, which
// catalog generation a resource belongs to, what the object does NOT carry —
// is a fact about the resource, so it is passed in rather than generalized.
const (
	longCustomerList = "Customers are the billing identity that subscriptions, invoices and\n" +
		"payment sources all hang off. Filters worth knowing:\n" +
		"`auto_collection[is]=on` for accounts charged automatically,\n" +
		"`email[is]=…` to find one person, `created_at[after]=<unix-seconds>` for\n" +
		"recent signups — Chargebee timestamps are UNIX seconds, never ISO strings."

	longCustomerGet = "Takes Chargebee's customer id, which on many sites is a value the calling\n" +
		"system supplied at creation rather than an opaque token. Returns balances,\n" +
		"`auto_collection` and the primary payment source, but NOT the customer's\n" +
		"subscriptions or invoices — those are separate lists filtered by\n" +
		"`customer_id[is]=<id>`."

	longCustomerCreate = "Fields go through repeatable `--param key=value`, including Chargebee's own\n" +
		"nested bracket form (`--param billing_address[city]=Berlin`). No field is\n" +
		"mandatory, so a customer with nothing but an id is valid. Passing\n" +
		"`--param id=<your-id>` sets the customer id instead of letting Chargebee\n" +
		"mint one, which is what makes creation idempotent against an external\n" +
		"system."

	longCustomerUpdate = "Only the `--param` fields sent are changed; everything else is left alone.\n" +
		"It reaches customer attributes only — Chargebee keeps the billing address\n" +
		"and payment methods on their own endpoints, so they cannot be changed from\n" +
		"here."

	longSubscriptionList = "A subscription is one customer's recurring commitment to a set of item\n" +
		"prices. `status[is]=active`, `status[in]=[\"active\",\"non_renewing\"]` and\n" +
		"`customer_id[is]=<id>` are the filters that matter. A cancelled\n" +
		"subscription stays in the list with `status` cancelled rather than\n" +
		"disappearing."

	longSubscriptionGet = "Returns the `subscription_items`, current term dates, `next_billing_at` and\n" +
		"`status`. That status matters: `non_renewing` means a cancellation is\n" +
		"SCHEDULED but the subscription is still active and still billing until the\n" +
		"term ends, which is not the same as `cancelled` or `paused`."

	longSubscriptionChange = "REPLACES the subscription's items with the `--item-price` set given — it is\n" +
		"not additive, so a current item left out is removed. Chargebee prorates by\n" +
		"default, meaning this call can raise an invoice or a credit note on the\n" +
		"spot; `--param prorate=false` suppresses that and\n" +
		"`--param end_of_term=true` defers the whole change to the next renewal."

	longSubscriptionCancel = "Cancels IMMEDIATELY by default: access ends now and Chargebee may issue a\n" +
		"credit note for the unused period. `--param end_of_term=true` instead\n" +
		"leaves it running to the end of the paid term in `non_renewing` status,\n" +
		"which is usually what a person means by \"cancel\". `subscription\n" +
		"reactivate` can restart a cancelled subscription, but it does not undo the\n" +
		"credits an immediate cancel already generated."

	longSubscriptionReactivate = "Restarts a CANCELLED subscription and begins a new billing term, charging\n" +
		"as that term starts rather than resuming the old one where it stopped. A\n" +
		"subscription that is merely `non_renewing` has not been cancelled yet, so\n" +
		"this is not the verb for it."

	longInvoiceList = "Invoices are raised by Chargebee and cannot be created here.\n" +
		"`status[is]=payment_due` finds what is owed, `status[is]=paid` what\n" +
		"settled, `customer_id[is]=<id>` scopes to one account. Amounts are in the\n" +
		"currency's minor unit, and `total`, `amount_paid` and `amount_due` diverge\n" +
		"on a partly paid invoice."

	longInvoiceGet = "Returns line items, applied credits and discounts, `amount_due`, and the\n" +
		"linked transactions and credit notes — enough to say not just what is owed\n" +
		"but why. The document itself is not here; `invoice pdf` mints a download\n" +
		"URL for that."

	longCreditNoteList = "Credit notes record money credited back to a customer. Chargebee issues\n" +
		"them from refunds, immediate cancellations and downgrade prorations —\n" +
		"nothing here creates one. `type[is]=refundable` separates cash-refundable\n" +
		"credit from adjustment credit, and `reference_invoice_id[is]=<id>` ties one\n" +
		"to the invoice it offsets."

	longCreditNoteGet = "Returns the line items, the invoice referenced, and how much of the note has\n" +
		"actually been refunded or allocated — `amount_available` is the part still\n" +
		"unused, which is what makes a credit note different from a refund receipt."

	longItemList = "Items are Product Catalog 2.0 products — `type` plan, addon or charge — and\n" +
		"carry NO price; the money lives on `item-price`. `type[is]=plan` narrows to\n" +
		"the subscribable ones. An empty list usually means the site is still on\n" +
		"Product Catalog 1.0, where `plan` is the equivalent surface."

	longItemGet = "Returns one item's type, status and metadata, and no price at all. A\n" +
		"subscription references an ITEM PRICE, so the useful follow-up is\n" +
		"`item-price list --filter \"item_id[is]=<id>\"`, whose ids are what\n" +
		"`subscription create --item-price` takes."

	longItemPriceList = "Item prices are what a subscription is actually built from: one item can\n" +
		"have several, differing by currency, billing period or pricing model. Their\n" +
		"ids (`basic-USD-monthly`) are exactly what `--item-price` takes on\n" +
		"`subscription create` and `subscription change`. Filter with\n" +
		"`item_id[is]=<id>` or `currency_code[is]=USD`."

	longItemPriceGet = "Returns the price, currency, billing period, and pricing model — flat,\n" +
		"per-unit, tiered or metered — plus any trial configuration. Worth reading\n" +
		"before subscribing someone: a metered price bills nothing until\n" +
		"`usage create` records quantities against it."

	longPlanList = "Plans are the PRODUCT CATALOG 1.0 surface. On a site migrated to PC 2.0\n" +
		"this list is empty, which is a catalog-generation fact rather than an\n" +
		"error — use `item` and `item-price` there. Subscriptions here are created\n" +
		"from item prices, so a plan id is not directly usable for a new one."

	longPlanGet = "Returns a legacy Product Catalog 1.0 plan's price, billing period and trial\n" +
		"settings. Only meaningful on a site still running PC 1.0."

	longTransactionList = "Transactions are the payment attempts behind invoices — captures, refunds,\n" +
		"and the failures. `type[is]=payment`, `status[is]=failure` and\n" +
		"`customer_id[is]=<id>` are the useful filters. A failed charge appears here\n" +
		"with the gateway's error code, which the invoice alone does not carry."

	longTransactionGet = "Returns one gateway attempt: amount, status, the gateway used, the masked\n" +
		"payment method and, on a failure, the gateway's own `error_code` and\n" +
		"`error_text` — the answer to \"why did this payment fail\"."

	longEventList = "Events are the billing activity stream: every state change Chargebee also\n" +
		"delivers to webhooks, retained for a limited window rather than forever.\n" +
		"`event_type[is]=subscription_cancelled` and\n" +
		"`occurred_at[after]=<unix-seconds>` are the filters that matter. Polling\n" +
		"this is how to notice changes made in the Chargebee UI or by dunning."

	longEventGet = "Returns one event with its `content`: a SNAPSHOT of the affected objects as\n" +
		"they were when it fired, not their current state. Re-read the resource\n" +
		"itself for what is true now."

	longUsageList = "Metered quantities already recorded against subscriptions' metered item\n" +
		"prices. Filter with `subscription_id[is]=<id>` and\n" +
		"`usage_date[after]=<unix-seconds>`. Reading is flat like this; RECORDING is\n" +
		"not — `usage create` is subscription-scoped."

	longPaymentSourceList = "The stored cards and bank accounts Chargebee charges. Only masked,\n" +
		"non-sensitive fields come back, and `customer_id[is]=<id>` scopes to one\n" +
		"customer. There is no create or delete verb: adding or removing a payment\n" +
		"method needs the customer's own consent flow, not an API call from here."
)

// resourceCommands builds the full grouped-by-resource tree plus the read-only
// GET escape hatch.
func (s *Service) resourceCommands(cfg reqConfig) []*cobra.Command {
	customer := s.readGroup(cfg, "customer", "Manage customers", "/customers", longCustomerList, longCustomerGet)
	customer.AddCommand(
		s.formWriteCmd(cfg, "create", "Create a customer", longCustomerCreate, http.MethodPost, "/customers", false),
		s.formWriteByIDCmd(cfg, "update", "Update a customer", longCustomerUpdate, "/customers/%s"),
	)

	subscription := s.readGroup(cfg, "subscription", "Manage subscriptions", "/subscriptions", longSubscriptionList, longSubscriptionGet)
	subscription.AddCommand(
		s.subscriptionCreateCmd(cfg),
		s.subscriptionChangeCmd(cfg),
		s.subscriptionActionCmd(cfg, "cancel", "Cancel a subscription", longSubscriptionCancel, "/subscriptions/%s/cancel_for_items"),
		s.subscriptionActionCmd(cfg, "reactivate", "Reactivate a subscription", longSubscriptionReactivate, "/subscriptions/%s/reactivate"),
	)

	invoice := s.readGroup(cfg, "invoice", "Read invoices", "/invoices", longInvoiceList, longInvoiceGet)
	invoice.AddCommand(s.invoicePDFCmd(cfg))

	usage := newGroup("usage", "Metered usage")
	usage.AddCommand(
		s.listCmd(cfg, "/usages", longUsageList),
		s.usageCreateCmd(cfg),
	)

	paymentSource := newGroup("payment-source", "Read payment instruments")
	paymentSource.AddCommand(s.listCmd(cfg, "/payment_sources", longPaymentSourceList))

	return []*cobra.Command{
		customer,
		subscription,
		invoice,
		s.readGroup(cfg, "credit-note", "Read credit notes", "/credit_notes", longCreditNoteList, longCreditNoteGet),
		s.readGroup(cfg, "item", "Read catalog items", "/items", longItemList, longItemGet),
		s.readGroup(cfg, "item-price", "Read item prices", "/item_prices", longItemPriceList, longItemPriceGet),
		s.readGroup(cfg, "plan", "Read plans (Product Catalog 1.0)", "/plans", longPlanList, longPlanGet),
		s.readGroup(cfg, "transaction", "Read payment transactions", "/transactions", longTransactionList, longTransactionGet),
		s.readGroup(cfg, "event", "Read the billing event stream", "/events", longEventList, longEventGet),
		usage,
		paymentSource,
		s.getCmd(cfg),
	}
}

// newGroup is a runnable command group: a bare group prints help; an unknown
// subcommand fails (cobra.NoArgs). Groups carry no side_effect annotation
// (design 318 (b)/(f)).
func newGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// readGroup is a resource group pre-populated with the shared list + get reads.
// listLong / getLong are the resource's own depth texts; the builders are
// shared, the prose is not.
func (s *Service) readGroup(cfg reqConfig, use, short, resourcePath, listLong, getLong string) *cobra.Command {
	group := newGroup(use, short)
	group.AddCommand(
		s.listCmd(cfg, resourcePath, listLong),
		s.getCmdForResource(cfg, resourcePath, getLong),
	)
	return group
}

// listCmd is the shared paged list read: --limit / --offset plus repeated
// --filter <field[op]=value> mapped verbatim to Chargebee bracket-operator query
// params.
func (s *Service) listCmd(cfg reqConfig, resourcePath, long string) *cobra.Command {
	var (
		limit   int
		offset  string
		filters []string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List resources",
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			if offset != "" {
				query.Set("offset", offset)
			}
			if err := applyFilters(query, filters); err != nil {
				return err
			}
			return s.read(cmd, cfg, resourcePath, query)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Chargebee caps at 100)")
	cmd.Flags().StringVar(&offset, "offset", "", "opaque pagination cursor (next_offset from a prior page)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "bracket-operator filter, e.g. status[is]=active (repeatable)")
	return cmd
}

// getCmdForResource is the shared single-object read: GET <resource>/{id}.
func (s *Service) getCmdForResource(cfg reqConfig, resourcePath, long string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one resource by id",
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.read(cmd, cfg, resourcePath+"/"+url.PathEscape(args[0]), nil)
		},
	}
}

// getCmd is the top-level read-only GET escape hatch for the long tail (quotes,
// estimates, orders, exports).
func (s *Service) getCmd(cfg reqConfig) *cobra.Command {
	var (
		path    string
		queries []string
	)
	cmd := &cobra.Command{
		Use:   "get --path <p> [--query k=v ...]",
		Short: "GET any Chargebee v2 path (read-only escape hatch)",
		Long: "Reaches the v2 resources that have no verb here — quotes, estimates,\n" +
			"orders, exports, gifts. `--path` must start with `/` and is sent\n" +
			"unchanged; `--query key=value` repeats and accepts the same\n" +
			"bracket-operator filters the `list` verbs take, e.g.\n" +
			"`--query \"status[is]=active\"`. GET only, deliberately: there is no write\n" +
			"escape hatch, so anything that changes billing state has to go through a\n" +
			"named verb.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !strings.HasPrefix(path, "/") {
				return &usageError{msg: "--path must start with '/'"}
			}
			query := url.Values{}
			for _, raw := range queries {
				key, value, ok := strings.Cut(raw, "=")
				if !ok {
					return &usageError{msg: fmt.Sprintf("--query %q must be key=value", raw)}
				}
				query.Add(key, value)
			}
			return s.read(cmd, cfg, path, query)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "v2 path to GET, e.g. /quotes")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter key=value (repeatable)")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

// formWriteCmd is a POST that takes flat form fields via repeated --param.
func (s *Service) formWriteCmd(cfg reqConfig, use, short, long, method, path string, requireParam bool) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			return s.write(cmd, cfg, method, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value (repeatable)")
	if requireParam {
		_ = cmd.MarkFlagRequired("param")
	}
	return cmd
}

// formWriteByIDCmd is a POST to <pathTemplate % id> with flat --param fields.
func (s *Service) formWriteByIDCmd(cfg reqConfig, use, short, long, pathTemplate string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			return s.write(cmd, cfg, http.MethodPost, fmt.Sprintf(pathTemplate, url.PathEscape(args[0])), form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value (repeatable)")
	return cmd
}

// subscriptionCreateCmd posts to the customer-scoped create-for-items path with
// the bracketed indexed subscription_items array plus flat --param fields.
func (s *Service) subscriptionCreateCmd(cfg reqConfig) *cobra.Command {
	var (
		customerID string
		itemPrices []string
		params     []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a subscription for a customer",
		Long: "Created against `--customer-id`, so the customer must exist first. This is\n" +
			"Product Catalog 2.0: a subscription is a SET OF ITEM PRICES, so\n" +
			"`--item-price id[:quantity]` is required and repeatable — `basic-USD:1\n" +
			"addon-USD` is two lines on one subscription. Extra fields go through\n" +
			"`--param` (`auto_collection`, `start_date`, `coupon_ids[0]`). With\n" +
			"auto-collection on, Chargebee raises AND CHARGES the first invoice as part\n" +
			"of this call, against the customer's stored payment source.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			if err := applyItemPrices(form, itemPrices); err != nil {
				return err
			}
			path := "/customers/" + url.PathEscape(customerID) + "/subscription_for_items"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer the subscription belongs to")
	cmd.Flags().StringArrayVar(&itemPrices, "item-price", nil, "item price as id[:quantity] (repeatable)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra form field key=value (repeatable)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("item-price")
	return cmd
}

// subscriptionChangeCmd updates a subscription's items (and optional flat fields).
func (s *Service) subscriptionChangeCmd(cfg reqConfig) *cobra.Command {
	var (
		itemPrices []string
		params     []string
	)
	cmd := &cobra.Command{
		Use:         "change <id>",
		Short:       "Change a subscription's items",
		Long:        longSubscriptionChange,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			if err := applyItemPrices(form, itemPrices); err != nil {
				return err
			}
			path := "/subscriptions/" + url.PathEscape(args[0]) + "/update_subscription_for_items"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&itemPrices, "item-price", nil, "item price as id[:quantity] (repeatable)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra form field key=value (repeatable)")
	return cmd
}

// subscriptionActionCmd is a by-id POST action (cancel/reactivate) with optional
// flat --param fields.
func (s *Service) subscriptionActionCmd(cfg reqConfig, use, short, long, pathTemplate string) *cobra.Command {
	return s.formWriteByIDCmd(cfg, use, short, long, pathTemplate)
}

// usageCreateCmd posts metered usage to the subscription-scoped usages path.
// There is no flat POST /usages, so the subscription id is required.
func (s *Service) usageCreateCmd(cfg reqConfig) *cobra.Command {
	var (
		subscriptionID string
		params         []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Record metered usage on a subscription",
		Long: "There is NO flat usage endpoint in Chargebee, so `--subscription-id` is\n" +
			"required and usage is always recorded against one subscription. The fields\n" +
			"that matter are `--param item_price_id=<metered price>` and\n" +
			"`--param quantity=<n>`, and that item price must be a metered one already\n" +
			"on the subscription or Chargebee rejects the call. Recorded usage feeds\n" +
			"the next invoice — this is billable data, not telemetry.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			path := "/subscriptions/" + url.PathEscape(subscriptionID) + "/usages"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringVar(&subscriptionID, "subscription-id", "", "subscription the usage is recorded against")
	cmd.Flags().StringArrayVar(&params, "param", nil, "usage form field key=value (repeatable)")
	_ = cmd.MarkFlagRequired("subscription-id")
	return cmd
}

// invoicePDFCmd issues the POST /invoices/{id}/pdf request, which returns a JSON
// download object (transient download_url + valid_till), not raw PDF bytes.
func (s *Service) invoicePDFCmd(cfg reqConfig) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:   "pdf <id>",
		Short: "Get a transient download URL for an invoice PDF",
		Long: "A POST that returns a JSON `download` object — a TRANSIENT `download_url`\n" +
			"plus `valid_till` — not PDF bytes, so the URL has to be fetched separately\n" +
			"and before it expires. `--param disposition_type=attachment` controls how\n" +
			"a browser treats it. It changes no invoice state and is safe to repeat.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			path := "/invoices/" + url.PathEscape(args[0]) + "/pdf"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value, e.g. disposition_type=attachment (repeatable)")
	return cmd
}

// read performs a GET and emits the provider JSON passthrough.
func (s *Service) read(cmd *cobra.Command, cfg reqConfig, path string, query url.Values) error {
	body, err := s.call(cmd.Context(), cfg, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// write performs a form-encoded POST and emits the provider JSON passthrough.
func (s *Service) write(cmd *cobra.Command, cfg reqConfig, method, path string, form url.Values) error {
	if form == nil {
		form = url.Values{}
	}
	body, err := s.call(cmd.Context(), cfg, method, path, nil, form)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// applyFilters maps repeated bracket-operator --filter values (status[is]=active)
// verbatim onto query params. The key already carries Chargebee's [op] suffix.
func applyFilters(query url.Values, filters []string) error {
	for _, raw := range filters {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || key == "" {
			return &usageError{msg: fmt.Sprintf("--filter %q must be field[op]=value", raw)}
		}
		query.Add(key, value)
	}
	return nil
}

// formFromParams splits repeated --param key=value into a form value set.
func formFromParams(params []string) (url.Values, error) {
	form := url.Values{}
	for _, raw := range params {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("--param %q must be key=value", raw)}
		}
		form.Add(key, value)
	}
	return form, nil
}

// applyItemPrices expands repeated id[:quantity] entries onto Chargebee's
// bracketed indexed subscription_items array
// (subscription_items[item_price_id][i], subscription_items[quantity][i]).
func applyItemPrices(form url.Values, itemPrices []string) error {
	for i, raw := range itemPrices {
		id, qty, hasQty := strings.Cut(raw, ":")
		if id == "" {
			return &usageError{msg: fmt.Sprintf("--item-price %q must be item_price_id[:quantity]", raw)}
		}
		form.Set(fmt.Sprintf("subscription_items[item_price_id][%d]", i), id)
		if hasQty {
			if _, err := strconv.Atoi(qty); err != nil {
				return &usageError{msg: fmt.Sprintf("--item-price %q quantity must be an integer", raw)}
			}
			form.Set(fmt.Sprintf("subscription_items[quantity][%d]", i), qty)
		}
	}
	return nil
}
