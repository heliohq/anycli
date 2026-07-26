package xero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// resourceCtx carries the resolved token and default tenant into each command's
// RunE closure, so the tree builder stays declarative.
type resourceCtx struct {
	svc           *Service
	token         string
	defaultTenant string
}

// tenantSelector returns the effective --tenant value: the flag if set, else the
// injected default (XERO_TENANT_ID), else empty (single-org auto-resolution).
func (rc *resourceCtx) tenantSelector(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetString("tenant"); strings.TrimSpace(v) != "" {
		return v
	}
	return rc.defaultTenant
}

// resolve turns the selector into a concrete tenantId to send in Xero-Tenant-Id.
func (rc *resourceCtx) resolve(ctx context.Context, cmd *cobra.Command) (string, error) {
	return rc.svc.resolveTenant(ctx, rc.token, rc.tenantSelector(cmd))
}

// addQueryFlag registers the repeatable --query k=v passthrough and returns the
// pointer to accumulate into.
func addQueryFlag(cmd *cobra.Command) *[]string {
	var q []string
	cmd.Flags().StringArrayVar(&q, "query", nil, "repeatable query parameter, key=value (e.g. --query where=... --query page=2)")
	return &q
}

// parseQuery turns []"key=value" into url.Values. A missing '=' is a usage error.
func parseQuery(pairs []string) (url.Values, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	v := url.Values{}
	for _, p := range pairs {
		k, val, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, &usageError{msg: fmt.Sprintf("--query %q must be key=value", p)}
		}
		v.Add(k, val)
	}
	return v, nil
}

// accountingPath joins the /api.xro/2.0 prefix with a resource path.
func accountingPath(resource string) string {
	return accountingPrefix + resource
}

// connectionsCmd lists the Xero organisations the token can act on. No tenant
// header; output is the /connections array verbatim so the AI can pick one.
func (rc *resourceCtx) connectionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connections",
		Short: "List connected Xero organisations (id, tenantId, name, type)",
		Long: "Lists the Xero organisations this login can act on; each entry's\n" +
			"`tenantId` is what `--tenant` takes. It is the one command that needs no\n" +
			"organisation of its own, so it still works while every other command is\n" +
			"exiting 2 for an unresolved one. A login with a single organisation never\n" +
			"needs it — the tenant resolves itself.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, connectionsPath, "", nil, nil)
			if err != nil {
				return err
			}
			return rc.svc.emitJSON(body)
		},
	}
}

// The Long texts below are handed to the shared listCmd / getCmd / writeCmd
// builders. They live next to those builders because it is the builder that
// fixes the HTTP method and the endpoint each text describes — what a
// collection response omits, which id a POST needs so it updates instead of
// creating, whether the endpoint pages at all.
const (
	longContactList = "Xero returns at most 100 contacts per page; continue with `--query\n" +
		"page=2`. `--query where=` takes Xero's filter syntax against contact\n" +
		"fields (`Name.Contains(\"Acme\")`, `EmailAddress!=null`,\n" +
		"`ContactStatus==\"ACTIVE\"`) and `--query order=Name` sorts.\n" +
		"`--query summaryOnly=true` drops the nested detail and is markedly faster\n" +
		"on a large organisation; archived contacts are excluded unless\n" +
		"`--query includeArchived=true` is passed."

	longContactGet = "Takes the `ContactID` GUID, or the organisation's own `ContactNumber` if\n" +
		"one was set. Unlike a `contact list` row this carries the full record —\n" +
		"addresses, phones, default account and tax codes, and the contact's\n" +
		"outstanding receivable and payable balances."

	longContactCreate = "PUT `{\"Contacts\":[{…}]}` through `--data` or `--file`. `Name` is the only\n" +
		"required field and must be unique within the organisation — a duplicate\n" +
		"comes back as a ValidationException, not a merge onto the existing\n" +
		"contact. The array accepts many contacts in one call."

	longContactUpdate = "POST, not PUT — Xero's verbs are inverted. The envelope MUST carry the\n" +
		"`ContactID` of the contact to change: without one Xero creates a new\n" +
		"contact instead of failing. Fields absent from the payload are left\n" +
		"alone, but collections such as `Addresses` and `Phones` are replaced\n" +
		"wholesale, so send every element that should survive."

	longInvoiceList = "Collection responses OMIT `LineItems` — Xero returns them only for a\n" +
		"single invoice, so switch to `invoice get` when the per-line amounts\n" +
		"matter. At most 100 invoices come back per page; continue with `--query\n" +
		"page=2`. Narrow with `--query Statuses=DRAFT,AUTHORISED`, `--query\n" +
		"ContactIDs=<guid>`, `--query where='Type==\"ACCPAY\"'` for bills, or\n" +
		"`--query where='Date>=DateTime(2026,01,01)'` — a date in a where clause\n" +
		"is a `DateTime(y,m,d)` call, not a quoted string."

	longInvoiceGet = "Accepts either the `InvoiceID` GUID or the human invoice number\n" +
		"(`INV-0042`). This is the only way to read an invoice's `LineItems`,\n" +
		"`Payments` and `CreditNotes`; the collection endpoint strips them.\n" +
		"`AmountDue` is what a later `payment create` may still settle."

	longInvoiceCreate = "PUT `{\"Invoices\":[{…}]}`. `Type` is required and picks the ledger side:\n" +
		"`ACCREC` is a sales invoice owed to the organisation, `ACCPAY` a bill it\n" +
		"owes. The contact is referenced by `ContactID`, each line by\n" +
		"`AccountCode` from `account list` and `TaxType` from `tax-rate list`.\n" +
		"`Status` defaults to DRAFT and a draft cannot be paid, so set\n" +
		"`\"Status\":\"AUTHORISED\"` when the invoice is meant to be payable.\n" +
		"`LineAmountTypes` decides whether `UnitAmount` already contains tax."

	longInvoiceUpdate = "POST with `InvoiceID` in the envelope; a payload without one creates a\n" +
		"new invoice. `LineItems` are replaced wholesale — send the complete set,\n" +
		"because an omitted line is a deleted line. An invoice that already has\n" +
		"payments applied can no longer have its lines or totals changed.\n" +
		"Cancelling is a status write rather than a delete: `\"Status\":\"DELETED\"`\n" +
		"for a DRAFT, `\"Status\":\"VOIDED\"` for an AUTHORISED invoice with no\n" +
		"payments against it."

	longPaymentList = "At most 100 payments per page; continue with `--query page=2`. Filter with\n" +
		"`--query where='Status==\"AUTHORISED\"'` or\n" +
		"`--query where='Date>=DateTime(2026,01,01)'`. Each entry links back to\n" +
		"the invoice it settles through `Invoice.InvoiceID`."

	longPaymentGet = "Takes the `PaymentID` GUID from `payment list`, or from the `Payments`\n" +
		"array that `invoice get` returns. The response carries the invoice,\n" +
		"contact and bank account the payment touched."

	longPaymentCreate = "PUT `{\"Payments\":[{\"Invoice\":{\"InvoiceID\":…},\"Account\":{\"Code\":…},\n" +
		"\"Amount\":…,\"Date\":\"2026-01-31\"}]}`. The invoice must be AUTHORISED — a\n" +
		"DRAFT cannot be paid — and `Amount` may not exceed its `AmountDue`. The\n" +
		"account must be one whose `EnablePaymentsToAccount` is true in\n" +
		"`account list`. The ledger moves immediately and nothing here reverses\n" +
		"it: deleting a payment is a status write this tool does not expose."

	longBankTxnList = "Bank transactions are spend/receive money entries recorded against a bank\n" +
		"account, not the imported bank statement lines they may later reconcile\n" +
		"against. Line items are omitted from a collection, as with invoices. At\n" +
		"most 100 per page; continue with `--query page=2`. Filter with\n" +
		"`--query where='Type==\"SPEND\"'` or on `BankAccount.AccountID`."

	longBankTxnGet = "Takes the `BankTransactionID` GUID. This is where `LineItems` appear —\n" +
		"the collection response omits them — along with the bank account the\n" +
		"money moved through and the transaction's reconciliation state."

	longBankTxnCreate = "PUT `{\"BankTransactions\":[{…}]}`. `Type` (`SPEND` or `RECEIVE`, plus the\n" +
		"`-OVERPAYMENT` and `-PREPAYMENT` variants), a `BankAccount` naming an\n" +
		"account of type BANK from `account list`, a `Contact` and at least one\n" +
		"line item are all required. This records a cash movement; it does not\n" +
		"reconcile an imported statement line."

	longAccountList = "The chart of accounts, returned in full — this endpoint does not page.\n" +
		"`Code` is the value writes reference: `AccountCode` on an invoice or\n" +
		"bank-transaction line, `Account.Code` on a payment. Narrow with\n" +
		"`--query where='Type==\"BANK\"'` or `Class==\"REVENUE\"`.\n" +
		"`EnablePaymentsToAccount` says whether `payment create` may settle into\n" +
		"an account."

	longItemList = "Products and services, returned in full — this endpoint does not page.\n" +
		"Invoice and bank-transaction lines reference an item by its `Code`, not\n" +
		"by `ItemID`. `SalesDetails` and `PurchaseDetails` hold the unit price,\n" +
		"account code and tax type a line inherits when it names the item."

	longItemGet = "Takes the `ItemID` GUID or the item `Code`. Returns the full record,\n" +
		"including `SalesDetails`, `PurchaseDetails` and — for a tracked item —\n" +
		"`QuantityOnHand` and `TotalCostPool`."

	longItemCreate = "PUT `{\"Items\":[{…}]}`. `Code` is required and unique per organisation.\n" +
		"An item is inventory-tracked only when `InventoryAssetAccountCode` is\n" +
		"set, and a tracked item then also requires the sales account code and\n" +
		"the purchase COGS account code; both detail blocks stay optional\n" +
		"otherwise."

	longItemUpdate = "POST with `ItemID` in the envelope; a payload without one creates a new\n" +
		"item rather than failing. Only the fields sent are changed, except a\n" +
		"nested `SalesDetails` or `PurchaseDetails` block, which is replaced as a\n" +
		"whole."

	longTaxRateList = "Tax rates are organisation- and region-specific and returned in full —\n" +
		"this endpoint does not page. A write references a rate by its `TaxType`\n" +
		"code (`OUTPUT2`, `INPUT2`, `NONE`, …) on a line item, never by `Name`, so\n" +
		"read the code here before composing an invoice or bank transaction.\n" +
		"`EffectiveRate` is the percentage actually applied."
)

// listCmd is a GET on a collection with --query passthrough (where, order, page…).
func (rc *resourceCtx) listCmd(use, short, long, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	q := addQueryFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		query, err := parseQuery(*q)
		if err != nil {
			return err
		}
		body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, accountingPath(resource), tenant, query, nil)
		if err != nil {
			return err
		}
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// getCmd is a GET on a single resource by id (or invoice number).
func (rc *resourceCtx) getCmd(use, short, long, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		id := strings.TrimSpace(args[0])
		if id == "" {
			return &usageError{msg: "empty id"}
		}
		path := accountingPath(resource) + "/" + url.PathEscape(id)
		body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, path, tenant, nil, nil)
		if err != nil {
			return err
		}
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// writeCmd is a create (PUT) or update (POST) on a collection. The body is the
// caller-supplied Xero JSON envelope, forwarded verbatim, from --data or --file.
func (rc *resourceCtx) writeCmd(use, short, long, method, resource string) *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&data, "data", "", "request body as a Xero JSON envelope (mutually exclusive with --file)")
	cmd.Flags().StringVar(&file, "file", "", "read the request body from a JSON file")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := readBody(data, file)
		if err != nil {
			return err
		}
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		body, err := rc.svc.call(cmd.Context(), rc.token, method, accountingPath(resource), tenant, nil, payload)
		if err != nil {
			return err
		}
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// emailCmd emails a sales invoice: POST /Invoices/{id}/Email (empty body).
func (rc *resourceCtx) emailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email <id>",
		Short: "Email a sales invoice to its contact",
		Long: "Sends the invoice through Xero's own mail, to the address on the\n" +
			"invoice's contact and using the organisation's branding theme. There is\n" +
			"no recipient, subject or body override, and no copy of the message comes\n" +
			"back. Xero rejects a DRAFT invoice, so authorise it first. A successful\n" +
			"send returns 204 with no body: nothing prints and the exit code is 0.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		id := strings.TrimSpace(args[0])
		if id == "" {
			return &usageError{msg: "empty invoice id"}
		}
		path := accountingPath("/Invoices") + "/" + url.PathEscape(id) + "/Email"
		body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodPost, path, tenant, nil, json.RawMessage(`{}`))
		if err != nil {
			return err
		}
		// Xero returns 204 No Content on a successful send; emitJSON no-ops on
		// an empty body, so nothing prints and the exit code stays 0.
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// reportName maps the CLI report words to Xero Reports endpoints.
var reportName = map[string]string{
	"pnl":              "ProfitAndLoss",
	"balance-sheet":    "BalanceSheet",
	"trial-balance":    "TrialBalance",
	"aged-receivables": "AgedReceivablesByContact",
	"aged-payables":    "AgedPayablesByContact",
}

// reportCmd is `report <name>` over GET /Reports/{Name} with --query passthrough
// (date, periods, timeframe, contactId for the aged reports, …).
func (rc *resourceCtx) reportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <pnl|balance-sheet|trial-balance|aged-receivables|aged-payables>",
		Short: "Pull a financial report",
		Long: "`--query` carries the report's own parameters: `date` for a\n" +
			"point-in-time report, `fromDate`/`toDate` for a span, `periods` with\n" +
			"`timeframe=MONTH|QUARTER|YEAR` to put comparative columns on `pnl`, and\n" +
			"`contactId=<guid>`, which `aged-receivables` and `aged-payables` REQUIRE.\n" +
			"Output is Xero's generic report shape — `Reports[0].Rows`, nested\n" +
			"row/cell objects in presentation order — not a flat table of figures.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	q := addQueryFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name, ok := reportName[strings.ToLower(strings.TrimSpace(args[0]))]
		if !ok {
			return &usageError{msg: fmt.Sprintf("unknown report %q; one of pnl|balance-sheet|trial-balance|aged-receivables|aged-payables", args[0])}
		}
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		query, err := parseQuery(*q)
		if err != nil {
			return err
		}
		body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, accountingPath("/Reports/"+name), tenant, query, nil)
		if err != nil {
			return err
		}
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// fetchCmd is the raw GET escape hatch: `fetch <path>` under /api.xro/2.0 so the
// AI can reach any Accounting resource (quotes, credit notes, journals, …) that
// the typed subcommands do not enumerate. A leading slash is tolerated.
func (rc *resourceCtx) fetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <path>",
		Short: "Raw GET under api.xro/2.0 (e.g. fetch CreditNotes --query where=...)",
		Long: "Reaches Accounting resources that have no typed subcommand:\n" +
			"`CreditNotes`, `Quotes`, `ManualJournals`, `TrackingCategories`,\n" +
			"`Journals`, `Users`, `BrandingThemes`. The argument is a path under\n" +
			"api.xro/2.0 — a full URL is rejected — and `--query` passes through as it\n" +
			"does elsewhere. Read only: there is no raw PUT or POST counterpart, so a\n" +
			"resource without a typed create command cannot be written at all.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	q := addQueryFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		raw := strings.TrimSpace(args[0])
		if raw == "" {
			return &usageError{msg: "empty path"}
		}
		if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			return &usageError{msg: "fetch takes a path under api.xro/2.0, not a full URL"}
		}
		tenant, err := rc.resolve(cmd.Context(), cmd)
		if err != nil {
			return err
		}
		query, err := parseQuery(*q)
		if err != nil {
			return err
		}
		body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, accountingPath("/"+strings.TrimPrefix(raw, "/")), tenant, query, nil)
		if err != nil {
			return err
		}
		return rc.svc.emitJSON(body)
	}
	return cmd
}

// orgGetCmd is GET /Organisation (no id).
func (rc *resourceCtx) orgGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the organisation's details",
		Long: "Returns the record of the organisation the call is targeting: legal and\n" +
			"trading name, `BaseCurrency`, country code, `FinancialYearEndDay` and\n" +
			"`Month`, the tax basis and the `OrganisationID`. Read `BaseCurrency`\n" +
			"before writing amounts — an invoice or payment with no explicit\n" +
			"`CurrencyCode` is taken in it. Takes no id; the organisation comes from\n" +
			"`--tenant`.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			tenant, err := rc.resolve(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			body, err := rc.svc.call(cmd.Context(), rc.token, http.MethodGet, accountingPath("/Organisation"), tenant, nil, nil)
			if err != nil {
				return err
			}
			return rc.svc.emitJSON(body)
		},
	}
}

// readBody resolves a write body from --data or --file (mutually exclusive) and
// validates it is JSON, so a malformed payload fails fast as a usage error
// (exit 2) instead of reaching Xero.
func readBody(data, file string) (json.RawMessage, error) {
	if data != "" && file != "" {
		return nil, &usageError{msg: "--data and --file are mutually exclusive"}
	}
	raw := data
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --file %s: %v", file, err)}
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "a request body is required (pass --data <json> or --file <path>)"}
	}
	if !json.Valid([]byte(raw)) {
		return nil, &usageError{msg: "request body is not valid JSON"}
	}
	return json.RawMessage(raw), nil
}
