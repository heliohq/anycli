package quickbooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// entitySpec describes one QuickBooks accounting entity exposed as a resource
// command group. command is both the CLI word and the lowercase REST path
// segment (e.g. "invoice"); queryName is the PascalCase entity used in the QBO
// query language (e.g. "Invoice"). canCreate/canSend gate the write verbs.
type entitySpec struct {
	command   string
	queryName string
	canCreate bool
	canSend   bool
	// Depth help for the generated verbs. They live on the spec rather than in
	// the shared builders because what a caller needs to know — which WHERE
	// fields matter, which refs a write must carry — is a fact about the
	// entity, not about the verb.
	listLong   string
	getLong    string
	createLong string
	sendLong   string
}

// entities is the fixed set of accounting resources the tool exposes, chosen by
// teammate task frequency (design §1). Reads run through the shared `query`
// grammar; get-by-id and create/send are the named verbs.
var entities = []entitySpec{
	{
		command: "customer", queryName: "Customer", canCreate: true,
		listLong: "Customers are the billing counterparties an invoice or payment points\n" +
			"at through `CustomerRef`. `Active = true` skips deactivated records and\n" +
			"`DisplayName like 'Acme%'` finds one by name — `DisplayName` is unique\n" +
			"per company and is what a person means by \"the customer\". Sub-customers\n" +
			"come back as ordinary rows carrying a `ParentRef`.",
		getLong: "`--id` is QuickBooks' numeric entity id, never the `DisplayName`; resolve\n" +
			"a name with `customer list --where \"DisplayName = 'Acme Ltd'\"` first.\n" +
			"Returns the balance, billing address, terms and the `SyncToken` that an\n" +
			"update through `customer create` requires.",
		createLong: "`DisplayName` is the only required field and must be UNIQUE in the\n" +
			"company — reusing an existing one fails with a duplicate-name Fault\n" +
			"rather than matching the existing customer. A sub-customer needs\n" +
			"`ParentRef` plus `Job: true`. Everything else follows QuickBooks' own\n" +
			"field shapes, so mirror a record from `customer get`.",
	},
	{
		command: "invoice", queryName: "Invoice", canCreate: true, canSend: true,
		listLong: "Invoices are money owed TO the company. `Balance > '0'` finds the unpaid\n" +
			"ones — note QuickBooks wants even numeric literals quoted — and\n" +
			"`TxnDate >= '2026-01-01'` bounds a period. Balance falls to zero when a\n" +
			"`payment` is applied, so a paid invoice is one with `Balance` 0, not a\n" +
			"missing row.",
		getLong: "`--id` is the numeric entity id, NOT the `DocNumber` printed on the\n" +
			"invoice — search by that with `invoice list --where \"DocNumber =\n" +
			"'1042'\"`. Returns every `Line`, the `CustomerRef`, `BillEmail`,\n" +
			"`Balance`, `EmailStatus` and the `SyncToken` an update needs.",
		createLong: "Requires a `CustomerRef` carrying the customer's id and at least one\n" +
			"`Line`, each with an `Amount`, a `DetailType` such as\n" +
			"`SalesItemLineDetail`, and the matching detail object naming an\n" +
			"`ItemRef` — `item list` resolves those ids. QuickBooks assigns\n" +
			"`DocNumber` unless the body sets one. Creating an invoice does NOT send\n" +
			"it; nothing reaches the customer until `invoice send`.",
		sendLong: "Emails the invoice to the customer as QuickBooks' own branded message and\n" +
			"sets its `EmailStatus` to `EmailSent`. That is a real message to a real\n" +
			"customer with no draft step and no unsend, and re-running sends it\n" +
			"again. `--to` overrides the recipient; omitted, QuickBooks uses the\n" +
			"invoice's own `BillEmail`, and an invoice carrying neither fails.",
	},
	{
		command: "bill", queryName: "Bill", canCreate: true,
		listLong: "Bills are money the company OWES a vendor — the accounts-payable mirror\n" +
			"of an invoice, recording an obligation with terms and a due date rather\n" +
			"than a paid expense. `Balance > '0'` finds the outstanding ones and\n" +
			"`VendorRef` narrows to one supplier.",
		getLong: "`--id` is the numeric entity id. Returns the lines, `VendorRef`,\n" +
			"`APAccountRef`, due date, balance and `SyncToken`. Bill lines normally\n" +
			"carry `AccountBasedExpenseLineDetail` rather than the item-based detail\n" +
			"an invoice uses, which is exactly the shape worth copying before\n" +
			"writing one.",
		createLong: "Requires a `VendorRef` and at least one `Line`. Bill lines normally use\n" +
			"`AccountBasedExpenseLineDetail` with an `AccountRef` naming the expense\n" +
			"account, and `account list` is where those ids come from. This records\n" +
			"a payable — it does not pay anything and moves no money.",
	},
	{
		command: "vendor", queryName: "Vendor", canCreate: true,
		listLong: "Vendors are the counterparties bills are owed to — the `VendorRef` a\n" +
			"bill points at. `DisplayName` is unique per company and is what a\n" +
			"person means by the supplier's name; `Active = true` skips deactivated\n" +
			"ones, and contractors tracked for 1099 purposes carry `Vendor1099 =\n" +
			"true`.",
		getLong: "`--id` is the numeric entity id; resolve a name through\n" +
			"`vendor list --where \"DisplayName = 'Acme Supply'\"`. Returns contact\n" +
			"details, the open balance, tax identifiers and the `SyncToken` an\n" +
			"update needs.",
		createLong: "`DisplayName` is required and must be UNIQUE in the company — reusing an\n" +
			"existing one fails with a duplicate Fault instead of matching the\n" +
			"vendor that already exists. Address, terms and tax id are optional and\n" +
			"follow QuickBooks' own field shapes.",
	},
	{
		command: "payment", queryName: "Payment", canCreate: true,
		listLong: "Payments are customer money RECEIVED, each optionally linked to the\n" +
			"invoices it settles. `CustomerRef` filters to one payer and\n" +
			"`UnappliedAmt > '0'` finds money taken in but not yet applied to any\n" +
			"invoice. Money going OUT to vendors is a different entity and is not\n" +
			"in this list.",
		getLong: "`--id` is the numeric entity id. The load-bearing field is\n" +
			"`Line[].LinkedTxn`, which names the invoices this payment was applied\n" +
			"to; `UnappliedAmt` is whatever is left sitting as a credit on the\n" +
			"customer.",
		createLong: "Requires a `CustomerRef` and a `TotalAmt`. To settle a specific invoice,\n" +
			"add a `Line` whose `LinkedTxn` names the invoice id with `TxnType:\n" +
			"\"Invoice\"` — without it the money is booked as an unapplied credit on\n" +
			"the customer and no invoice balance moves. Applying a payment reduces\n" +
			"the invoice's `Balance` in the real books.",
	},
	{
		command: "account", queryName: "Account",
		listLong: "The company's chart of accounts — the ledger buckets every transaction\n" +
			"posts to. `AccountType = 'Expense'` or `Classification = 'Revenue'`\n" +
			"narrows it, and the ids here are what an `AccountRef` on a bill or\n" +
			"expense line points at. There is no create verb for accounts: the\n" +
			"chart is maintained in QuickBooks itself.",
		getLong: "`--id` is the numeric entity id; find one from a human account name with\n" +
			"`account list --where \"Name = 'Office Supplies'\"`. Returns the\n" +
			"`AccountType`, `AccountSubType`, `CurrentBalance` and active flag.",
	},
	{
		command: "item", queryName: "Item",
		listLong: "Items are the products and services an invoice line can bill for — the\n" +
			"`ItemRef` inside a `SalesItemLineDetail`. `Type = 'Service'` or\n" +
			"`'Inventory'` narrows the list, and stock-tracked items carry\n" +
			"`QtyOnHand`. There is no create verb: items are set up in QuickBooks\n" +
			"itself.",
		getLong: "`--id` is the numeric entity id. Returns the name, type, unit price,\n" +
			"income and expense account references and, for inventory items, the\n" +
			"quantity on hand — the fields an invoice line needs to reference it\n" +
			"correctly.",
	},
}

// newGroupCmd is a runnable command group: a bare group prints help, but an
// unknown subcommand fails (cobra skips Args validation on non-runnable
// commands, which would exit 0 for an agent typo).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// newCompanyCmd builds `company get` → GET /companyinfo/<realmId>. CompanyInfo
// is itself company-scoped, so the resource id is the realmId.
func (c *client) newCompanyCmd() *cobra.Command {
	group := newGroupCmd("company", "Company identity / health check")
	get := &cobra.Command{
		Use:   "get",
		Short: "Fetch this company's CompanyInfo",
		Long: "Returns the CompanyInfo record for the connected realm: legal and trade\n" +
			"name, address, fiscal-year start month and QuickBooks edition. Since the\n" +
			"realm is fixed at connect time and never appears in a command, this is the\n" +
			"only way to confirm WHICH company's books are about to be read or written —\n" +
			"and the cheapest check that the connection still works.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.call(cmd.Context(), http.MethodGet, "companyinfo/"+url.PathEscape(c.realm), nil, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	group.AddCommand(get)
	return group
}

// newQueryCmd builds `query --sql "<QBO SQL>"` → GET /query?query=<sql>. This
// is the read workhorse: one verb covers most read intents (design §1).
func (c *client) newQueryCmd() *cobra.Command {
	var sql string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run a QuickBooks query (e.g. \"select * from Invoice where Balance > '0'\")",
		Long: "QuickBooks' SQL-LIKE grammar, and the single call that answers most read\n" +
			"questions. It is not SQL: no joins, one entity per statement, `count(*)`\n" +
			"as the only aggregate, PascalCase entity names, and every literal\n" +
			"single-quoted INCLUDING numbers (`Balance > '0'`). Pagination is written\n" +
			"into the statement itself as `startposition N maxresults M`. Only entities\n" +
			"QuickBooks exposes to the query service are addressable — the per-resource\n" +
			"`list` verbs are this same call with flags.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(sql) == "" {
				return &usageError{msg: "query requires --sql"}
			}
			q := url.Values{}
			q.Set("query", sql)
			body, err := c.call(cmd.Context(), http.MethodGet, "query", q, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&sql, "sql", "", "QuickBooks query statement (required)")
	return cmd
}

// newReportCmd builds `report get --name <ReportName>` → GET /reports/<name>.
// Common date controls (--start-date/--end-date/--date-macro) plus a repeatable
// --param key=value for any other report-specific parameter.
func (c *client) newReportCmd() *cobra.Command {
	group := newGroupCmd("report", "Financial reports (ProfitAndLoss, BalanceSheet, AgedReceivables, …)")
	var name, startDate, endDate, dateMacro string
	var params []string
	get := &cobra.Command{
		Use:   "get",
		Short: "Fetch a named report",
		Long: "`--name` is a QuickBooks report id in PascalCase — `ProfitAndLoss`,\n" +
			"`BalanceSheet`, `AgedReceivables`, `AgedPayables`, `CashFlow`,\n" +
			"`GeneralLedger`. The period comes from `--start-date` / `--end-date`\n" +
			"(YYYY-MM-DD) or from `--date-macro` (\"This Fiscal Year\", \"Last Month\");\n" +
			"with neither, QuickBooks applies its own default period rather than\n" +
			"covering all time, so read the period back off the response `Header`.\n" +
			"`--param key=value` repeats for report-specific options such as accounting\n" +
			"method or summarisation column. The result is a nested row/column tree,\n" +
			"not a flat table.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return &usageError{msg: "report get requires --name"}
			}
			q := url.Values{}
			if startDate != "" {
				q.Set("start_date", startDate)
			}
			if endDate != "" {
				q.Set("end_date", endDate)
			}
			if dateMacro != "" {
				q.Set("date_macro", dateMacro)
			}
			for _, p := range params {
				key, value, ok := strings.Cut(p, "=")
				if !ok || strings.TrimSpace(key) == "" {
					return &usageError{msg: fmt.Sprintf("--param %q must be key=value", p)}
				}
				q.Set(key, value)
			}
			body, err := c.call(cmd.Context(), http.MethodGet, "reports/"+url.PathEscape(name), q, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	get.Flags().StringVar(&name, "name", "", "report name, e.g. ProfitAndLoss (required)")
	get.Flags().StringVar(&startDate, "start-date", "", "report start date (YYYY-MM-DD)")
	get.Flags().StringVar(&endDate, "end-date", "", "report end date (YYYY-MM-DD)")
	get.Flags().StringVar(&dateMacro, "date-macro", "", "date range macro, e.g. \"This Fiscal Year\"")
	get.Flags().StringArrayVar(&params, "param", nil, "additional report parameter key=value (repeatable)")
	group.AddCommand(get)
	return group
}

// newEntityCmd builds one accounting-resource group (customer/invoice/…) with
// list/get and, per spec, create/send verbs.
func (c *client) newEntityCmd(spec entitySpec) *cobra.Command {
	group := newGroupCmd(spec.command, "Manage "+spec.command+" records")
	group.AddCommand(c.newEntityListCmd(spec), c.newEntityGetCmd(spec))
	if spec.canCreate {
		group.AddCommand(c.newEntityCreateCmd(spec))
	}
	if spec.canSend {
		group.AddCommand(c.newInvoiceSendCmd(spec))
	}
	return group
}

// newEntityListCmd builds `<entity> list` as a thin wrapper over the query
// grammar: SELECT * FROM <Entity> [WHERE ..] [STARTPOSITION n] [MAXRESULTS n].
// QBO paginates inside the query language, not via header links (design §2).
func (c *client) newEntityListCmd(spec entitySpec) *cobra.Command {
	var where string
	var maxResults, startPosition int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + spec.command + " records",
		Long:        spec.listLong,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			stmt := "select * from " + spec.queryName
			if w := strings.TrimSpace(where); w != "" {
				stmt += " where " + w
			}
			if startPosition > 0 {
				stmt += " startposition " + strconv.Itoa(startPosition)
			}
			if maxResults > 0 {
				stmt += " maxresults " + strconv.Itoa(maxResults)
			}
			q := url.Values{}
			q.Set("query", stmt)
			body, err := c.call(cmd.Context(), http.MethodGet, "query", q, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&where, "where", "", "QBO WHERE clause (without the WHERE keyword)")
	cmd.Flags().IntVar(&maxResults, "max", 0, "MAXRESULTS page size")
	cmd.Flags().IntVar(&startPosition, "start-position", 0, "STARTPOSITION 1-based offset")
	return cmd
}

// newEntityGetCmd builds `<entity> get --id <id>` → GET /<entity>/<id>.
func (c *client) newEntityGetCmd(spec entitySpec) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Fetch one " + spec.command + " by id",
		Long:        spec.getLong,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(id) == "" {
				return &usageError{msg: spec.command + " get requires --id"}
			}
			body, err := c.call(cmd.Context(), http.MethodGet, spec.command+"/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "entity id (required)")
	return cmd
}

// newEntityCreateCmd builds `<entity> create --json-body <json>` → POST
// /<entity>. QBO models create and update as full/sparse upserts on the same
// POST, so the caller supplies the raw QBO entity JSON (design §2).
func (c *client) newEntityCreateCmd(spec entitySpec) *cobra.Command {
	var jsonBody string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create or upsert a " + spec.command + " from raw QBO entity JSON",
		Long:        spec.createLong,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONBody(jsonBody)
			if err != nil {
				return err
			}
			body, err := c.call(cmd.Context(), http.MethodPost, spec.command, nil, payload)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "QBO entity JSON object (required)")
	return cmd
}

// newInvoiceSendCmd builds `invoice send --id <id> [--to <email>]` → POST
// /invoice/<id>/send[?sendTo=<email>]. Omitting --to uses the invoice's own
// BillEmail (QBO behavior).
func (c *client) newInvoiceSendCmd(spec entitySpec) *cobra.Command {
	var id, to string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Email a " + spec.command + " to the customer",
		Long:        spec.sendLong,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(id) == "" {
				return &usageError{msg: spec.command + " send requires --id"}
			}
			q := url.Values{}
			if strings.TrimSpace(to) != "" {
				q.Set("sendTo", to)
			}
			body, err := c.call(cmd.Context(), http.MethodPost, spec.command+"/"+url.PathEscape(id)+"/send", q, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "invoice id (required)")
	cmd.Flags().StringVar(&to, "to", "", "recipient email (defaults to the invoice BillEmail)")
	return cmd
}

// decodeJSONBody parses the required --json-body flag into a generic object.
func decodeJSONBody(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "create requires --json-body (a QBO entity JSON object)"}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--json-body is not a valid JSON object: %v", err)}
	}
	return payload, nil
}
