package lemonsqueezy

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// resourceGroups assembles the full grouped-by-resource command tree. Each
// resource is a cobra group whose leaves are the shared CRUD verbs plus any
// resource-specific actions (refund, generate-invoice, current-usage, cancel).
func (s *Service) resourceGroups(token string) []*cobra.Command {
	return []*cobra.Command{
		group("store", "Stores",
			s.list(token, "/stores"), s.getOne(token, "/stores")),
		group("product", "Products",
			s.list(token, "/products"), s.getOne(token, "/products")),
		group("variant", "Product variants",
			s.list(token, "/variants"), s.getOne(token, "/variants")),
		group("price", "Prices",
			s.list(token, "/prices"), s.getOne(token, "/prices")),
		group("file", "Digital-download files",
			s.list(token, "/files"), s.getOne(token, "/files")),
		group("order", "Orders",
			s.list(token, "/orders"), s.getOne(token, "/orders"),
			s.refund(token, "/orders"), s.generateInvoice(token, "/orders")),
		group("order-item", "Order items",
			s.list(token, "/order-items"), s.getOne(token, "/order-items")),
		group("customer", "Customers",
			s.list(token, "/customers"), s.getOne(token, "/customers"),
			s.create(token, "/customers"), s.update(token, "/customers")),
		group("subscription", "Subscriptions",
			s.list(token, "/subscriptions"), s.getOne(token, "/subscriptions"),
			s.update(token, "/subscriptions"), s.cancel(token, "/subscriptions")),
		group("subscription-invoice", "Subscription invoices",
			s.list(token, "/subscription-invoices"), s.getOne(token, "/subscription-invoices"),
			s.refund(token, "/subscription-invoices"), s.generateInvoice(token, "/subscription-invoices")),
		group("subscription-item", "Subscription items",
			s.list(token, "/subscription-items"), s.getOne(token, "/subscription-items"),
			s.update(token, "/subscription-items"), s.currentUsage(token, "/subscription-items")),
		group("usage-record", "Usage records",
			s.list(token, "/usage-records"), s.getOne(token, "/usage-records"),
			s.create(token, "/usage-records")),
		group("discount", "Discounts",
			s.list(token, "/discounts"), s.getOne(token, "/discounts"),
			s.create(token, "/discounts"), s.remove(token, "/discounts")),
		group("license-key", "License keys",
			s.list(token, "/license-keys"), s.getOne(token, "/license-keys"),
			s.update(token, "/license-keys")),
		group("license-key-instance", "License key instances",
			s.list(token, "/license-key-instances"), s.getOne(token, "/license-key-instances")),
		group("checkout", "Checkouts",
			s.list(token, "/checkouts"), s.getOne(token, "/checkouts"),
			s.create(token, "/checkouts")),
		group("webhook", "Webhooks",
			s.list(token, "/webhooks"), s.getOne(token, "/webhooks"),
			s.create(token, "/webhooks"), s.update(token, "/webhooks"),
			s.remove(token, "/webhooks")),
	}
}

// The verbs below are built once and reused across seventeen resources, so
// their Longs are built per resource path rather than written once: "delete"
// means retire a discount code and stop a webhook, "refund" means a purchase on
// /orders and a renewal on /subscription-invoices, and a list of prices needs a
// different warning than a list of stores.

// resourceListNote is the per-resource clause appended to a `list` Long. A
// resource with nothing non-obvious to add has no entry.
var resourceListNote = map[string]string{
	"/stores": "\nEvery other resource is scoped to a store, so `--filter store_id=<id>` is\n" +
		"the usual first narrowing and this is where that id comes from.",
	"/products": "\nA product is the catalogue entry; what a buyer actually purchases is a\n" +
		"`variant` of it, and what they are charged is a `price` on that variant.",
	"/variants": "\nVariants belong to a product and are what a checkout is created against.\n" +
		"`--filter product_id=<id>` narrows to one product.",
	"/prices": "\nA variant accumulates price rows over time instead of one row being edited\n" +
		"in place, so when several come back for the same variant the newest is the\n" +
		"live one. `--filter variant_id=<id>` narrows.",
	"/files": "\nFiles are the digital downloads attached to a variant. Read-only: there is\n" +
		"no upload verb in this tool.",
	"/orders": "\nOne-off purchases and the FIRST charge of a subscription both appear here.\n" +
		"Later renewal charges do not — those are `subscription-invoice` rows.",
	"/order-items": "\nOne row per variant inside an order. `--filter order_id=<id>` narrows to a\n" +
		"single order's lines.",
	"/customers": "\nCustomers belong to a store and are identified by email, so\n" +
		"`--filter email=<address>` is the way in from an address alone.",
	"/subscriptions": "\nA row's status is on the record, and `cancelled` does NOT mean access has\n" +
		"ended — a cancelled subscription stays valid until its `ends_at`.",
	"/subscription-invoices": "\nThe recurring charges behind subscriptions, one row per billing cycle.\n" +
		"Separate from `order`, and this is where a renewal gets refunded.",
	"/subscription-items": "\nItems are the priced lines of a subscription and what usage-based billing\n" +
		"meters against.",
	"/usage-records": "\nUsage records report metered quantities against a subscription item;\n" +
		"`--filter subscription_item_id=<id>` narrows to one.",
	"/license-keys": "\nThe account-side view of issued keys. The customer-facing license\n" +
		"activation API is a different surface and is not exposed by this tool.",
	"/license-key-instances": "\nOne row per activation of a key on a device.\n" +
		"`--filter license_key_id=<id>` narrows to a single key's activations.",
	"/checkouts": "\nCheckouts are hosted payment links. The `url` attribute is the deliverable,\n" +
		"and a checkout can carry an expiry, so an old one is not necessarily still\n" +
		"usable.",
	"/webhooks": "\nEach row carries the URL, the subscribed event list and the signing secret\n" +
		"used to verify deliveries.",
}

func longList(path string) string {
	return "Paging is `--page` / `--page-size`, which become JSON:API `page[number]`\n" +
		"and `page[size]`. `--filter key=value` is repeatable and becomes\n" +
		"`filter[key]=value`. `--include a,b` embeds the related resources in the\n" +
		"same response under `included`, which is one call instead of a follow-up\n" +
		"`get` per row." + resourceListNote[path]
}

func longGetOne(path string) string {
	long := "Takes the resource id as a positional argument. `--include a,b` embeds\n" +
		"related resources under `included` in the same response — cheaper than a\n" +
		"second call, and the only way to see a relationship's contents rather than\n" +
		"just its id."
	switch path {
	case "/subscriptions":
		long += "\n`--include order` ties the subscription back to the purchase that started\n" +
			"it; `--include subscription-items` lists its priced lines."
	case "/orders":
		long += "\n`--include order-items,customer` returns the purchased lines and the buyer\n" +
			"in the same document."
	}
	return long
}

// longCreate, longUpdate and longRemove are keyed by path because the shared
// builders serve resources whose creates and deletes mean very different
// things.
var longCreate = map[string]string{
	"/customers": "`--data` is required and is a whole JSON:API document — `{\"data\":{\"type\":\n" +
		"\"customers\",\"attributes\":{...},\"relationships\":{\"store\":...}}}`. The store\n" +
		"relationship is what scopes the customer; without it the call fails.",
	"/usage-records": "`--data` is required and reports a metered quantity against a subscription\n" +
		"item, named through the document's relationships. Reported usage is BILLED,\n" +
		"and there is no idempotency key here — sending the same record twice bills\n" +
		"the customer twice.",
	"/discounts": "`--data` is required and creates a discount code that is live for anyone\n" +
		"who knows it as soon as the call succeeds. Scope it with the store and, when\n" +
		"it should not apply to the whole catalogue, the variant relationships.",
	"/checkouts": "`--data` is required and must name at least the `store` and `variant`\n" +
		"relationships; custom pricing, prefilled buyer details and an expiry go in\n" +
		"`attributes`. The deliverable is the `url` on the created resource — that\n" +
		"link is what a buyer is sent to, and no money moves until they use it.",
	"/webhooks": "`--data` is required and carries the store relationship, the destination\n" +
		"`url`, the `events` to subscribe to and the `secret` Lemon Squeezy signs\n" +
		"deliveries with. Events start flowing to that URL immediately.",
}

var longUpdate = map[string]string{
	"/customers": "`--data` is required, and the document's `type` and `id` have to match the\n" +
		"resource being patched. Only the attributes present are changed.",
	"/subscriptions": "This is everything `subscription cancel` cannot do: resuming a cancelled\n" +
		"subscription, swapping the plan by pointing at a different variant, moving\n" +
		"the billing anchor, or pausing. A mid-cycle plan change is prorated by Lemon\n" +
		"Squeezy unless the document says otherwise, so it can charge or credit the\n" +
		"customer immediately. `--data` is required.",
	"/subscription-items": "Changes the quantity on a priced line, which is how seat-based billing is\n" +
		"adjusted. Lemon Squeezy prorates the difference against the current cycle,\n" +
		"so this moves money as well as state. `--data` is required.",
	"/license-keys": "Changes a key's activation limit, expiry or disabled state. Disabling takes\n" +
		"effect for the customer's next validation, not at their next renewal.\n" +
		"`--data` is required.",
	"/webhooks": "Changes the destination URL, the subscribed events or the signing secret.\n" +
		"Rotating the secret breaks signature verification for any receiver still\n" +
		"using the old one. `--data` is required.",
}

var longRemove = map[string]string{
	"/discounts": "Destroys the discount code. Any link or campaign already sent out that\n" +
		"references it stops applying, and there is no restore verb.",
	"/webhooks": "Stops deliveries immediately. Events that fire afterwards are not queued\n" +
		"for a webhook created later, so the gap is unrecoverable from this API.",
}

// longRefund and longInvoice are keyed by path because orders and subscription
// invoices are the two halves of a purchase's life and refunding the wrong one
// is the standing mistake.
var longRefund = map[string]string{
	"/orders": "Refunds a one-off purchase or the first charge of a subscription. Omitting\n" +
		"`--data` refunds the FULL order; a partial refund passes the amount in cents\n" +
		"inside a JSON:API document. Money moves and the call cannot be undone. A\n" +
		"renewal charge is not refundable here — that is `subscription-invoice\n" +
		"refund`.",
	"/subscription-invoices": "Refunds one billing cycle of a subscription, which is where every renewal\n" +
		"charge lives; the original purchase is refunded with `order refund` instead.\n" +
		"Omitting `--data` refunds the full invoice, and a partial refund passes the\n" +
		"amount in cents. Refunding does not cancel anything — the subscription keeps\n" +
		"renewing until `subscription cancel`.",
}

func longInvoice(path string) string {
	long := "Invoice fields are QUERY parameters, not a JSON body: `--param key=value`\n" +
		"is repeatable and accepts name, address, city, state, zip_code, country and\n" +
		"notes. The response carries a signed download URL under `meta.urls` rather\n" +
		"than the PDF itself, and a signed URL is short-lived — fetch it promptly."
	if path == "/subscription-invoices" {
		long += "\nThis invoices one subscription billing cycle; the original purchase is\n" +
			"`order invoice`."
	}
	return long
}

// group is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it.
func group(use, short string, subs ...*cobra.Command) *cobra.Command {
	g := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	g.AddCommand(subs...)
	return g
}

// list is the shared collection GET verb with JSON:API paging/filter/include
// flat flags.
func (s *Service) list(token, path string) *cobra.Command {
	var page, pageSize int
	var filters []string
	var include string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List (GET " + path + ")",
		Long:        longList(path),
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := listQuery(page, pageSize, filters, include)
			if err != nil {
				return err
			}
			return s.get(cmd.Context(), token, path, q)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (page[number])")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "items per page (page[size])")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "filter as key=value (repeatable → filter[key]=value)")
	cmd.Flags().StringVar(&include, "include", "", "comma-separated related resources to include")
	return cmd
}

// getOne is the shared single-resource GET verb.
func (s *Service) getOne(token, path string) *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one by id (GET " + path + "/{id})",
		Long:        longGetOne(path),
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.get(cmd.Context(), token, path+"/"+url.PathEscape(args[0]), includeQuery(include))
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "comma-separated related resources to include")
	return cmd
}

// create is the shared collection POST verb taking a raw JSON:API document.
func (s *Service) create(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create (POST " + path + ")",
		Long:        longCreate[path],
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			if payload == nil {
				return &usageError{msg: "--data is required (a JSON:API document, e.g. {\"data\":{\"type\":...}})"}
			}
			return s.send(cmd.Context(), token, http.MethodPost, path, nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON:API request document")
	return cmd
}

// update is the shared single-resource PATCH verb taking a raw JSON:API
// document.
func (s *Service) update(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update by id (PATCH " + path + "/{id})",
		Long:        longUpdate[path],
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			if payload == nil {
				return &usageError{msg: "--data is required (a JSON:API document)"}
			}
			return s.send(cmd.Context(), token, http.MethodPatch, path+"/"+url.PathEscape(args[0]), nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON:API request document")
	return cmd
}

// remove is the shared single-resource DELETE verb (used by discounts and
// webhooks, where DELETE means destroy).
func (s *Service) remove(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete by id (DELETE " + path + "/{id})",
		Long:        longRemove[path],
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.send(cmd.Context(), token, http.MethodDelete, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// cancel is the DELETE verb for subscriptions, where DELETE means "cancel"
// (the subscription stays valid through its grace period) rather than destroy.
func (s *Service) cancel(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a subscription (DELETE " + path + "/{id})",
		Long: "DELETE means CANCEL here, not destroy: the row moves to `cancelled` but the\n" +
			"customer keeps access until `ends_at` and billing simply stops renewing.\n" +
			"Nothing is refunded. To end access immediately, or to resume a\n" +
			"subscription already cancelled, use `subscription update` with the\n" +
			"appropriate attributes instead.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.send(cmd.Context(), token, http.MethodDelete, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// refund issues a refund against an order or subscription invoice
// (POST {path}/{id}/refund). --data is optional; omitting it issues a full
// refund, otherwise it carries the partial amount.
func (s *Service) refund(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "refund <id>",
		Short:       "Issue a refund (POST " + path + "/{id}/refund)",
		Long:        longRefund[path],
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			return s.send(cmd.Context(), token, http.MethodPost, path+"/"+url.PathEscape(args[0])+"/refund", nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "optional JSON:API document with a partial refund amount")
	return cmd
}

// generateInvoice generates a downloadable invoice
// (POST {path}/{id}/generate-invoice). Its fields are query parameters
// (name/address/city/…), passed as repeatable --param key=value.
func (s *Service) generateInvoice(token, path string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         "invoice <id>",
		Short:       "Generate a downloadable invoice (POST " + path + "/{id}/generate-invoice)",
		Long:        longInvoice(path),
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := paramQuery(params)
			if err != nil {
				return err
			}
			return s.send(cmd.Context(), token, http.MethodPost, path+"/"+url.PathEscape(args[0])+"/generate-invoice", q, nil)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "invoice field as key=value (repeatable): name, address, city, state, zip_code, country, notes")
	return cmd
}

// currentUsage reads a subscription item's current usage
// (GET /subscription-items/{id}/current-usage).
func (s *Service) currentUsage(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:   "current-usage <id>",
		Short: "Retrieve a subscription item's current usage (GET " + path + "/{id}/current-usage)",
		Long: "Reads how much metered usage has accrued against one subscription item in\n" +
			"the CURRENT billing period — what the next invoice will charge for, not a\n" +
			"lifetime total. Takes no flags. Reporting new usage is\n" +
			"`usage-record create`.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.get(cmd.Context(), token, path+"/"+url.PathEscape(args[0])+"/current-usage", nil)
		},
	}
}
