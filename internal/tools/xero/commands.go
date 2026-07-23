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
		Use:         "connections",
		Short:       "List connected Xero organisations (id, tenantId, name, type)",
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

// listCmd is a GET on a collection with --query passthrough (where, order, page…).
func (rc *resourceCtx) listCmd(use, short, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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
func (rc *resourceCtx) getCmd(use, short, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
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
func (rc *resourceCtx) writeCmd(use, short, method, resource string) *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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
		Use:         "email <id>",
		Short:       "Email a sales invoice to its contact",
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
		Use:         "report <pnl|balance-sheet|trial-balance|aged-receivables|aged-payables>",
		Short:       "Pull a financial report",
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
		Use:         "fetch <path>",
		Short:       "Raw GET under api.xro/2.0 (e.g. fetch CreditNotes --query where=...)",
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
		Use:         "get",
		Short:       "Get the organisation's details",
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
