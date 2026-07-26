package sage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newListCmd builds a `list` leaf: GET <path> with --page / --items-per-page
// query params and the resolved X-Business header. Output is Sage's list
// envelope ($items / $total / $next) verbatim, so the caller continues by
// re-requesting the next page.
// The Long texts below are passed into the shared newListCmd / newGetCmd /
// newCreateCmd builders. They live next to those builders because it is the
// builder that fixes the endpoint and the HTTP method each one describes.
const (
	longBusinessList = "The one command that needs no `--business`: it enumerates the businesses\n" +
		"this login can act on, and each `id` is what `--business` takes\n" +
		"everywhere else. Several businesses on one login is the normal case, and\n" +
		"Sage never says that a call fell through to the lead business — so read\n" +
		"this before anything that writes."

	longBusinessGet = "Takes a business id from `business list` and returns that business's own\n" +
		"record: name, country, base currency, financial year end and address.\n" +
		"The country matters before any write — it decides which tax fields a\n" +
		"contact or invoice envelope has to carry."

	longContactList = "Customers and suppliers share one collection and are told apart by\n" +
		"`contact_type_ids` (`CUSTOMER`, `VENDOR`); a contact can be both. Paged\n" +
		"like every list here. The `id` on a row is what a `sales_invoice` or\n" +
		"`contact_payment` envelope references as `contact_id`."

	longContactGet = "Takes a contact id and returns the full record — addresses, persons, tax\n" +
		"details, credit terms and outstanding balance — where a list row is a\n" +
		"summary. Check `contact_type_ids` before invoicing: a sales invoice\n" +
		"belongs to a CUSTOMER contact, a bill to a VENDOR one."

	longContactCreate = "`--body` is the complete envelope, `{\"contact\":{…}}`, and is validated as\n" +
		"JSON before the call. `name` and `contact_type_ids` (`[\"CUSTOMER\"]`,\n" +
		"`[\"VENDOR\"]`, or both) are the minimum; a tax-registered contact usually\n" +
		"also needs `main_address`. Nothing deduplicates, so a second contact\n" +
		"with the same name is created rather than merged. There is no contact\n" +
		"update command — a correction goes through\n" +
		"`fetch --method PUT --path /contacts/<id>`."

	longSalesInvoiceList = "Customer invoices: money owed TO the business. Supplier bills are a\n" +
		"different collection, `purchase-invoice list`. Rows carry\n" +
		"`total_amount` and `outstanding_amount`, and it is the outstanding\n" +
		"figure that a `contact-payment create` settles."

	longSalesInvoiceGet = "Takes a sales-invoice id and returns the whole document, `invoice_lines`\n" +
		"included. `outstanding_amount` is what a receipt should be allocated\n" +
		"against, and `status` says whether Sage still considers the invoice\n" +
		"unpaid. Amounts are in the business's own currency unless the invoice\n" +
		"names another."

	longSalesInvoiceCreate = "`--body` wraps a `sales_invoice` object needing at least `contact_id`,\n" +
		"`date` and `invoice_lines`; each line wants a `ledger_account_id` from\n" +
		"`ledger-account list` and a `tax_rate_id` from `tax-rate list`, both of\n" +
		"which are specific to this business. Pass `--business` EXPLICITLY —\n" +
		"without it the invoice posts to the user's lead business, and no update\n" +
		"or delete command here can take it back. The invoice posts unpaid:\n" +
		"recording the money is a separate `contact-payment create` allocated\n" +
		"against this invoice's id."

	longPurchaseInvoiceList = "Supplier bills: money the business OWES. The mirror of\n" +
		"`sales-invoice list`, and read-only — entering a bill has no typed\n" +
		"command and goes through\n" +
		"`fetch --method POST --path /purchase_invoices`."

	longPurchaseInvoiceGet = "Takes a purchase-invoice id and returns the full bill with its lines and\n" +
		"outstanding amount. Paying it is a `contact-payment create` carrying a\n" +
		"supplier-payment `transaction_type_id` and allocated against this id."

	longContactPaymentCreate = "Both customer receipts and supplier payments go through this one\n" +
		"endpoint; there is no per-invoice payment path. `--body` wraps a\n" +
		"`contact_payment` object whose `transaction_type_id` sets the direction\n" +
		"(`CUSTOMER_RECEIPT` for money in, the supplier-payment type for money\n" +
		"out), plus `contact_id`, a `bank_account_id` from `bank-account list`,\n" +
		"`date` and `total_amount`. Allocating it against an invoice means\n" +
		"`allocated_artefacts[]` with the invoice id as `artefact_id` and the\n" +
		"amount to apply — leave that out and the payment sits on the contact as\n" +
		"unallocated cash. The ledger moves immediately and nothing here\n" +
		"reverses it."

	longLedgerAccountList = "The chart of accounts for the selected business. A `ledger_account_id`\n" +
		"from here is required on every invoice line, and the ids are\n" +
		"business-specific — one never carries across a `--business` switch,\n" +
		"even for the same nominal code. `nominal_code` and `displayed_as` are\n" +
		"the human-recognisable fields."

	longBankAccountList = "Bank, credit-card and cash accounts with their balances — the fastest\n" +
		"read of the business's cash position. An `id` here is what a\n" +
		"`contact_payment` envelope names as `bank_account_id`, deciding which\n" +
		"account the money lands in."

	longBankAccountGet = "Takes a bank-account id and returns the one account with its balance,\n" +
		"currency and type. The balance is Sage's ledger position, not a live\n" +
		"feed from the bank, so it reflects what has been recorded rather than\n" +
		"what has cleared."

	longProductList = "Stocked or catalogued products the business sells. An invoice line may\n" +
		"reference one, but need not: a line with its own description,\n" +
		"`ledger_account_id` and amount is equally valid. Read-only here —\n" +
		"creating a product goes through `fetch --method POST --path /products`."

	longServiceList = "Non-stock offerings the business bills for, the counterpart to\n" +
		"`product list` and the usual source for a consulting or subscription\n" +
		"line. Each carries the default ledger account and tax rate a line\n" +
		"inherits when it names the service. Read-only here too."

	longTaxRateList = "The tax rates configured for THIS business, which differ by country —\n" +
		"UK VAT, US sales tax and FR TVA are separate sets, not translations of\n" +
		"one another. An invoice line references a `tax_rate_id` from here, and\n" +
		"a rate id belonging to another business is rejected. `percentage` and\n" +
		"the agency fields say which rate a given line should carry."
)

func (s *Service) newListCmd(token, use, path, short, long string) *cobra.Command {
	var page, itemsPerPage int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&itemsPerPage, "items-per-page", 0, "max items per page")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodGet, withPaging(path, page, itemsPerPage), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newGetCmd builds a `get <id>` leaf: GET <path>/<id> with the resolved
// X-Business header. Output is the resource JSON verbatim.
func (s *Service) newGetCmd(token, use, path, short, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if id == "" {
			return &usageError{msg: "empty id"}
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodGet, path+"/"+url.PathEscape(id), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCreateCmd builds a `create` leaf: POST <path> with a verbatim --body JSON
// payload (the caller supplies the exact Sage resource envelope, e.g.
// {"contact":{…}} / {"sales_invoice":{…}} / {"contact_payment":{…}}) and the
// resolved X-Business header. Output is the created resource JSON verbatim.
func (s *Service) newCreateCmd(token, use, path, short, long string) *cobra.Command {
	var bodyFlag string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&bodyFlag, "body", "", "request body: the full Sage resource JSON envelope (required)")
	_ = cmd.MarkFlagRequired("body")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := parseBody(bodyFlag)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newFetchCmd is the top-level generic passthrough: `fetch --method --path
// [--body]` reaches any v3.1 resource not modeled above, on the same Bearer +
// X-Business path. --path is the resource path below the v3.1 base (a leading
// slash is optional). Annotated side-effecting because --method can mutate.
func (s *Service) newFetchCmd(token string) *cobra.Command {
	var method, path, bodyFlag string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Call any Sage Accounting v3.1 endpoint directly",
		Long: "The passthrough to any v3.1 resource with no typed command — journals,\n" +
			"addresses, credit notes, attachments and some forty others. `--path` is\n" +
			"relative to the v3.1 base with an optional leading slash; `--method` is\n" +
			"GET, POST, PUT, DELETE or PATCH and defaults to GET; `--body` carries\n" +
			"the envelope for the mutating ones. This is also the ONLY route to an\n" +
			"update or a delete, since the typed commands cover reads and three\n" +
			"creates. `--business` scoping and the rate limits apply exactly as\n" +
			"elsewhere, and nothing about the payload is checked beyond it being\n" +
			"valid JSON.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&method, "method", http.MethodGet, "HTTP method (GET|POST|PUT|DELETE)")
	cmd.Flags().StringVar(&path, "path", "", "resource path below /v3.1 (e.g. /contacts or contacts) (required)")
	cmd.Flags().StringVar(&bodyFlag, "body", "", "request body JSON (for POST/PUT)")
	_ = cmd.MarkFlagRequired("path")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		m := strings.ToUpper(strings.TrimSpace(method))
		switch m {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		default:
			return &usageError{msg: fmt.Sprintf("--method must be one of GET|POST|PUT|DELETE|PATCH, got %q", method)}
		}
		p := strings.TrimSpace(path)
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		var payload any
		if strings.TrimSpace(bodyFlag) != "" {
			parsed, err := parseBody(bodyFlag)
			if err != nil {
				return err
			}
			payload = parsed
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), m, p, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// parseBody validates a --body JSON payload on parse. An empty value is a
// fail-fast usage error (create/fetch-with-body require a body); invalid JSON
// is likewise a usage error.
func parseBody(val string) (json.RawMessage, error) {
	if strings.TrimSpace(val) == "" {
		return nil, &usageError{msg: "--body is required and must be valid JSON"}
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(val), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--body is not valid JSON: %v", err)}
	}
	return raw, nil
}
