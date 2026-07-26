package zohobooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// stringFlag pairs a resource-specific CLI flag with the Books query param it
// maps to. Values are passed through verbatim, so status views like
// Status.Overdue and _contains/_startswith field variants work without a
// client-side allowlist.
type stringFlag struct{ name, query, usage string }

// requireOrg validates the persistent --organization-id flag on an org-scoped
// command. A missing value is a usage error (exit 2) that points the agent at
// `org list` — there is no "pick a default org" fallback.
func requireOrg(orgID string) (string, error) {
	if strings.TrimSpace(orgID) == "" {
		return "", &usageError{msg: "--organization-id is required; run `zoho-books org list` to discover your organization ids"}
	}
	return strings.TrimSpace(orgID), nil
}

// newListCmd builds `<resource> list` — GET /books/v3/{path}?organization_id=…
// with the shared pagination flags (--page, --per-page) plus any
// resource-specific filters. The provider JSON envelope (incl. page_context)
// is printed verbatim.
// The Long texts below are handed to the shared newListCmd / newGetCmd /
// newCreateCmd builders. They live beside those builders because the builder
// is what fixes the Books endpoint and HTTP method each text describes.
const (
	longContactList = "Customers and vendors share this one collection and are told apart by\n" +
		"`--contact-type customer|vendor`. `--search-text` is a free-text match\n" +
		"over the contact's name and email; `--filter-by` takes a Books status\n" +
		"view such as `Status.Active`. A row's `contact_id` is what\n" +
		"`invoice create` and `estimate create` reference as `customer_id`, and\n" +
		"what `bill list --vendor-id` filters on."

	longContactGet = "`--id` is a required flag carrying the `contact_id` from `contact list`.\n" +
		"Returns the full record — billing and shipping addresses, contact\n" +
		"persons, payment terms, currency and the outstanding receivable — where\n" +
		"a list row is a summary. Note the contact's `currency_code`: Books\n" +
		"raises that contact's invoices in it rather than in the organization's\n" +
		"currency."

	longContactCreate = "`--data` is a FLAT JSON object —\n" +
		"`{\"contact_name\":\"Acme Inc\",\"contact_type\":\"customer\"}` — checked\n" +
		"locally for being a single object before the call. `contact_name` is\n" +
		"required and Books rejects a name that already exists in the\n" +
		"organization. `contact_type` decides whether the record can be invoiced\n" +
		"(`customer`) or billed (`vendor`). Keep the returned `contact_id`;\n" +
		"nothing in this tool updates a contact afterwards."

	longInvoiceList = "Receivables. `--status` takes a raw Books status (`draft`, `sent`,\n" +
		"`overdue`, `paid`, `void`) and `--filter-by` a status view\n" +
		"(`Status.Overdue`, `Status.Sent`, `Status.Paid`); both pass through\n" +
		"untouched, so a value Books does not recognise comes back as a non-zero\n" +
		"`code` rather than an empty list. `--customer-id` narrows to one\n" +
		"customer. Rows carry `balance` — what is still owed — next to `total`."

	longInvoiceGet = "`--id` is the `invoice_id`, a long numeric string, not the human\n" +
		"`invoice_number` printed on the document. Adds what list rows leave\n" +
		"out: `line_items`, the tax breakdown, the payments applied so far and\n" +
		"the invoice's shareable URL. `balance` is the amount still outstanding."

	longInvoiceCreate = "`--data` is a flat JSON object with `customer_id` and `line_items`, each\n" +
		"line either naming an `item_id` from `item list` or carrying its own\n" +
		"`name`, `rate` and `quantity`. Nothing wraps it — Books takes no\n" +
		"`{\"data\":[…]}` envelope, unlike Zoho CRM. Books creates the invoice as\n" +
		"a draft, and this tool has no send, email or mark-paid command, so the\n" +
		"customer sees nothing until someone acts in Books itself."

	longEstimateList = "Estimates are quotes: the pre-invoice document, with no effect on the\n" +
		"ledger until someone turns it into an invoice. `--filter-by` takes a\n" +
		"status view (`Status.Sent`, `Status.Accepted`, `Status.Declined`) and\n" +
		"`--customer-id` narrows to one customer. Conversion is not available\n" +
		"here; an accepted estimate still needs a separate `invoice create`."

	longEstimateGet = "`--id` is the `estimate_id`. Returns the whole quote — `line_items`,\n" +
		"totals, expiry date and current status. Acceptance does not create an\n" +
		"invoice by itself and nothing in this tool converts one, so the invoice\n" +
		"has to be raised separately."

	longEstimateCreate = "`--data` is a flat JSON object with `customer_id` and `line_items`;\n" +
		"lines may name an `item_id` or spell out `name`, `rate` and `quantity`.\n" +
		"Creating an estimate posts nothing to the ledger and emails nobody,\n" +
		"which makes it the safe way to price work before an invoice exists.\n" +
		"Books assigns the estimate number unless the payload sets one."

	longItemList = "The catalogue behind line items: name, `rate`, description, tax and the\n" +
		"account each is booked to. `--search-text` is the only filter. Look an\n" +
		"item up before composing `invoice create` or `estimate create` — a line\n" +
		"that names an `item_id` inherits the rate and tax configured here\n" +
		"instead of hard-coding a price that will drift."

	longItemGet = "`--id` is the `item_id`. Returns one item's full definition — rate,\n" +
		"description, SKU, unit, tax association and `status`. An item whose\n" +
		"status is inactive should not go on a new invoice line."

	longBillList = "Payables: what the organization owes its vendors, the mirror of\n" +
		"`invoice list`. `--vendor-id` narrows to one vendor and\n" +
		"`--filter-by Status.Overdue` to what is late. Read-only — there is no\n" +
		"bill create and no vendor-payment command in this tool, so entering or\n" +
		"paying a bill happens in Books."

	longBillGet = "`--id` is the `bill_id`. Returns the bill with its line items, due date\n" +
		"and `balance`, the amount still owed to the vendor. Read-only, like the\n" +
		"rest of the bill group."

	longPaymentList = "Customer payments — money received, and the record of which invoice each\n" +
		"receipt settled. `--customer-id` is the only filter: there is no date\n" +
		"or amount filter, so a \"payments this month\" question means paging and\n" +
		"narrowing on the client side. Vendor payments are not exposed by this\n" +
		"tool at all."

	longPaymentGet = "`--id` is the `payment_id` from `payment list`. Returns the receipt with\n" +
		"its amount, date, payment mode, the account it landed in and the\n" +
		"invoices it was applied against."

	longExpenseList = "Recorded expenses, on the payables side. `--filter-by` takes a status\n" +
		"view such as `Status.Unbilled` — expenses that could still be re-billed\n" +
		"to a customer — or `Status.Billed`. `--search-text` matches free text.\n" +
		"Rows carry the expense account and whether the expense is billable."

	longExpenseGet = "`--id` is the `expense_id`. Returns the single expense with its amount,\n" +
		"tax, the account it was booked to, the account it was paid through and\n" +
		"the customer it is billable to, if any."

	longExpenseCreate = "`--data` is a flat JSON object needing at least `account_id` (the\n" +
		"expense account) and `amount`; `paid_through_account_id` names the bank\n" +
		"or credit account the money left. Both ids come from the organization's\n" +
		"chart of accounts, which this tool cannot list — take them from Books\n" +
		"or from an existing row in `expense list`. Adding `customer_id` with\n" +
		"`is_billable` makes the expense re-billable. It posts to the ledger\n" +
		"immediately and nothing here reverses it."
)

func (s *Service) newListCmd(token string, orgID *string, path, long string, extra []stringFlag) *cobra.Command {
	var page, perPage int
	vals := make(map[string]*string, len(extra))
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + path,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	for _, f := range extra {
		p := new(string)
		vals[f.query] = p
		cmd.Flags().StringVar(p, f.name, "", f.usage)
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "records per page (default/max 200)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("organization_id", org)
		for key, p := range vals {
			if strings.TrimSpace(*p) != "" {
				q.Set(key, *p)
			}
		}
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+path+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newGetCmd builds `<resource> get` — GET /books/v3/{path}/{id}?organization_id=…
func (s *Service) newGetCmd(token string, orgID *string, path, long string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one " + strings.TrimSuffix(path, "s") + " by id",
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		q := url.Values{}
		q.Set("organization_id", org)
		p := "/" + path + "/" + url.PathEscape(strings.TrimSpace(id)) + "?" + q.Encode()
		body, err := s.call(cmd.Context(), token, http.MethodGet, p, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCreateCmd builds `<resource> create` — POST /books/v3/{path}?organization_id=…
// with --data sent as the raw request body. Books create endpoints take a flat
// JSON object (line-item-bearing creates put line_items inside --data), NOT a
// {"data":[…]} wrapper — this is the Books/CRM divergence.
func (s *Service) newCreateCmd(token string, orgID *string, path, long string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create one " + strings.TrimSuffix(path, "s"),
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object for the new record (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		raw, err := rawObject(data)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("organization_id", org)
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/"+path+"?"+q.Encode(), raw)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newOrgCmd builds the top-level `org` group and its `list` verb —
// GET /books/v3/organizations, the one endpoint that takes NO organization_id
// and yields the ids every other command requires.
func (s *Service) newOrgCmd(token string) *cobra.Command {
	org := newGroupCmd("org", "Discover the organizations this login can operate in")
	list := &cobra.Command{
		Use:   "list",
		Short: "List organizations (no --organization-id; discovers the ids)",
		Long: "The only command that takes no `--organization-id`, and the one that\n" +
			"produces them: each entry's `organization_id` is what every other\n" +
			"command requires. A Zoho login can own several Books organizations and\n" +
			"the id is neither in the token nor guessable, so this is the first call\n" +
			"of a session. Each entry also carries the organization's\n" +
			"`currency_code`, time zone and country, which decide what an amount\n" +
			"written there actually means.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/organizations", nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	org.AddCommand(list)
	return org
}

// rawObject validates that --data is a non-empty single JSON object and returns
// the original bytes for verbatim passthrough. Empty is a usage error; invalid
// JSON is a usage error; a non-object JSON value is rejected.
func rawObject(val string) ([]byte, error) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil, &usageError{msg: "--data is required (a single JSON object)"}
	}
	var probe any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
	}
	if _, ok := probe.(map[string]any); !ok {
		return nil, &usageError{msg: "--data must be a single JSON object"}
	}
	return []byte(trimmed), nil
}
