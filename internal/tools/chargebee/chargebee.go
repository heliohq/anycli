// Package chargebee is the built-in Chargebee service: a non-interactive cobra
// tree over the Chargebee Billing v2 REST surface
// (https://{site}.chargebee.com/api/v2). Auth is HTTP Basic with the site API
// key as the username and an empty password. Reads are JSON; writes are
// application/x-www-form-urlencoded with Chargebee's bracketed nested-array
// encoding. Every command emits the provider's native JSON on stdout; a non-2xx
// surfaces Chargebee's api_error_code/message, and a 401/403 rejects the
// credential.
package chargebee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvAPIKey and EnvSite are the env vars the credential binding injects
// (definitions/tools/chargebee.json). The site is not a secret but is required
// to build the per-site host; the key is the Basic-auth username.
const (
	EnvAPIKey = "CHARGEBEE_API_KEY"
	EnvSite   = "CHARGEBEE_SITE"
)

// Service implements the built-in Chargebee tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the per-site API base; empty = built from CHARGEBEE_SITE.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// reqConfig is the resolved per-invocation request context captured by every
// RunE closure: the API base and the Basic-auth key.
type reqConfig struct {
	base   string
	apiKey string
}

// Execute runs one chargebee subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing flags, bad enums, unknown
// subcommands, invalid input) are exit 2; runtime/API errors (Chargebee non-2xx,
// transport failure) are exit 1. Errors render to stderr — as a JSON envelope
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	apiKey := env[EnvAPIKey]
	if apiKey == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	site := env[EnvSite]
	if s.BaseURL == "" && site == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvSite + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.BaseURL
	if base == "" {
		base = baseURLFromSite(site)
	}

	root := s.newRoot(reqConfig{base: base, apiKey: apiKey})
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
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// baseURLFromSite builds the per-site Chargebee v2 API base. The site subdomain
// is the customer's Chargebee site name (e.g. acme-test).
func baseURLFromSite(site string) string {
	return "https://" + site + ".chargebee.com/api/v2"
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// missing-credential checks).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>,
// "api_error_code":<Chargebee code or omitted>}}.
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
		if apiErr.code != "" {
			payload["api_error_code"] = apiErr.code
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(cfg reqConfig) *cobra.Command {
	root := &cobra.Command{
		Use: "chargebee",
		Long: "Chargebee Billing v2 against ONE site. The site and its API key were\n" +
			"captured at connect time and are applied automatically, so no command names\n" +
			"them and one connection reads and writes exactly one site's billing data.\n" +
			"\n" +
			"Responses are Chargebee's native JSON. A list is\n" +
			"`{\"list\": [{\"<resource>\": {…}}, …], \"next_offset\": \"<cursor>\"}` — note each\n" +
			"row is wrapped in a single-key object — and a single read is\n" +
			"`{\"<resource>\": {…}}`. Page by feeding `next_offset` back as `--offset`; it\n" +
			"is an OPAQUE cursor, not a row count, and Chargebee caps `--limit` at 100.\n" +
			"\n" +
			"Filters use Chargebee's bracket-operator syntax, passed through verbatim and\n" +
			"repeatable: `--filter \"status[is]=active\"`,\n" +
			"`--filter \"id[in]=[\\\"cust_1\\\",\\\"cust_2\\\"]\"`. Timestamps inside them are UNIX\n" +
			"SECONDS.\n" +
			"\n" +
			"Writes are form-encoded: flat fields through repeatable\n" +
			"`--param key=value`, subscription lines through `--item-price\n" +
			"id[:quantity]`, which the tool expands into Chargebee's indexed bracket\n" +
			"encoding.\n" +
			"\n" +
			"The catalog has two generations. A Product Catalog 2.0 site subscribes to\n" +
			"ITEM PRICES (`item`, `item-price`), which is what `subscription create` and\n" +
			"`subscription change` take; `plan` is the legacy PC 1.0 surface and is empty\n" +
			"on a 2.0 site.\n" +
			"\n" +
			"Billing writes move real money: creating a subscription can charge a card,\n" +
			"changing one can prorate into an invoice or a credit note, and none of it is\n" +
			"reversible from here.",
		Short:         "Chargebee subscription billing (v2 REST) built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(s.resourceCommands(cfg)...)
	return root
}

// NewCommandTree returns the full command tree built with an empty request
// config for dry-run parsing and traversal (tools.Service seam, design 318).
// The config is only captured by RunE closures, which are never run on this
// tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(reqConfig{}) }
