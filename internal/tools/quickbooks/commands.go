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
}

// entities is the fixed set of accounting resources the tool exposes, chosen by
// teammate task frequency (design §1). Reads run through the shared `query`
// grammar; get-by-id and create/send are the named verbs.
var entities = []entitySpec{
	{command: "customer", queryName: "Customer", canCreate: true},
	{command: "invoice", queryName: "Invoice", canCreate: true, canSend: true},
	{command: "bill", queryName: "Bill", canCreate: true},
	{command: "vendor", queryName: "Vendor", canCreate: true},
	{command: "payment", queryName: "Payment", canCreate: true},
	{command: "account", queryName: "Account"},
	{command: "item", queryName: "Item"},
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
		Use:         "get",
		Short:       "Fetch this company's CompanyInfo",
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
		Use:         "query",
		Short:       "Run a QuickBooks query (e.g. \"select * from Invoice where Balance > '0'\")",
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
		Use:         "get",
		Short:       "Fetch a named report",
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
