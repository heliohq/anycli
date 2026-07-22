// Package shopify is the built-in Shopify service: a verb-first cobra tree over
// the Shopify GraphQL Admin API. Since 2025-04-01 all new public Shopify apps
// must be built exclusively on the GraphQL Admin API (the REST Admin API is
// legacy), so every subcommand compiles to a single
// POST /admin/api/<version>/graphql.json call under the hood — the AI never
// hand-writes GraphQL except through the explicit `graphql` passthrough.
//
// Auth is the Shopify-specific X-Shopify-Access-Token header (not
// Authorization: Bearer). The base host is per-store, injected as SHOPIFY_STORE
// (a {shop}.myshopify.com domain); the access token is injected as
// SHOPIFY_ACCESS_TOKEN. Mutations return HTTP 200 with a userErrors array on
// validation failure, so the service treats a non-empty userErrors as an
// exit-1 failure rather than a silent no-op success.
package shopify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultAPIVersion is the pinned Shopify Admin API version. Shopify versions
// are date-based and quarterly with 12-month support; pin it (never `latest`)
// so the request shape stays stable, and expose --api-version for forward-compat.
const DefaultAPIVersion = "2026-07"

// Credential env vars injected by definitions/tools/shopify.json.
const (
	EnvAccessToken = "SHOPIFY_ACCESS_TOKEN"
	EnvStore       = "SHOPIFY_STORE"
)

// accessTokenHeader is the Shopify-specific auth header (not Authorization).
const accessTokenHeader = "X-Shopify-Access-Token"

// Service implements the built-in Shopify tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the full GraphQL endpoint; empty = built from Store +
	// APIVersion. Tests point it at an httptest server.
	BaseURL string
	// Store overrides SHOPIFY_STORE (the {shop}.myshopify.com host); empty =
	// read from env. Only used when BaseURL is empty.
	Store string
	// APIVersion overrides DefaultAPIVersion.
	APIVersion string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one shopify subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Shopify
// non-2xx, GraphQL errors, non-empty userErrors, transport failure) are exit 1.
// Errors render to stderr — JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	// Absent credentials are a runtime/environment failure — the connection was
	// never injected — not a caller-fixable usage error. Render them as an
	// apiError so the emitted kind ("api" = the API/runtime category) agrees
	// with the exit 1 below; a usageError would emit kind "usage" (exit 2) and
	// disagree.
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	store := s.Store
	if store == "" {
		store = env[EnvStore]
	}
	// When BaseURL is set (tests), the store host is not required to build the
	// endpoint; otherwise the per-store host is mandatory.
	if s.BaseURL == "" && normalizeStore(store) == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: EnvStore + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, store)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return execution.Failure(err), nil
	}
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (e.g. the pre-parse missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// newRoot builds the grouped-by-resource cobra tree. shop info, product, order,
// customer, inventory hang under resource groups; graphql is the top-level raw
// passthrough escape hatch.
func (s *Service) newRoot(token, store string) *cobra.Command {
	root := &cobra.Command{
		Use:           "shopify",
		Short:         "Shopify store admin (GraphQL Admin API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")
	pf.String("api-version", "", "Shopify Admin API version (default "+DefaultAPIVersion+")")

	c := &client{svc: s, token: token, store: store}

	product := newGroupCmd("product", "Manage products")
	product.AddCommand(
		c.newProductListCmd(),
		c.newProductGetCmd(),
		c.newProductCreateCmd(),
		c.newProductUpdateCmd(),
	)
	order := newGroupCmd("order", "Manage orders")
	order.AddCommand(
		c.newOrderListCmd(),
		c.newOrderGetCmd(),
		c.newOrderUpdateCmd(),
	)
	customer := newGroupCmd("customer", "Manage customers")
	customer.AddCommand(
		c.newCustomerListCmd(),
		c.newCustomerGetCmd(),
		c.newCustomerCreateCmd(),
		c.newCustomerUpdateCmd(),
	)
	inventory := newGroupCmd("inventory", "Inventory levels")
	inventory.AddCommand(
		c.newInventoryLevelsCmd(),
		c.newInventoryAdjustCmd(),
	)
	shop := newGroupCmd("shop", "Store identity and health")
	shop.AddCommand(c.newShopInfoCmd())

	root.AddCommand(product, order, customer, inventory, shop, c.newGraphQLCmd())
	return root
}

// newGroupCmd is a runnable command group (help-only RunE). cobra skips Args
// validation on non-runnable commands, so making the group runnable restores
// "unknown subcommand fails" behavior instead of a false exit-0.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with empty credentials for
// dry-run parsing/traversal (tools.Service seam, design 318). RunE closures
// capture the token/store but are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("", "") }

// normalizeStore turns a bare shop name or full myshopify domain into the
// canonical {shop}.myshopify.com host, or "" when the input is empty. A value
// that already carries a dot is treated as a full host (trimmed of scheme/path).
func normalizeStore(store string) string {
	s := strings.TrimSpace(store)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return ""
	}
	if strings.Contains(s, ".") {
		return s
	}
	return s + ".myshopify.com"
}
