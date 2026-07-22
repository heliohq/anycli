// Package quickbooks is the built-in QuickBooks Online service: a cobra tree
// over Intuit's company-scoped Accounting API v3 plus the Reports API. Every
// call hangs off /v3/company/<realmId>/ and carries a pinned minorversion;
// the read workhorse is `query` (QBO's SQL-like grammar) with named get/create
// verbs for the common entities. QuickBooks fails with a non-2xx status and a
// {"Fault":{"Error":[…]}} body — every call surfaces the code and detail.
package quickbooks

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

// Env vars the credential bindings inject (definitions/tools/quickbooks.json).
const (
	// EnvAccessToken is the OAuth 2.0 bearer token.
	EnvAccessToken = "QUICKBOOKS_ACCESS_TOKEN"
	// EnvRealmID is the company id (realmId) captured at connect time; it is
	// required in every company-scoped URL.
	EnvRealmID = "QUICKBOOKS_REALM_ID"
	// EnvEnvironment optionally selects the sandbox host for the L2 dev
	// harness; absent/"production" means the live API. It is NOT a credential
	// (there is no Helio credential source for it), so it is read from the
	// process environment rather than the resolver-supplied credential map:
	// the harness operator exports it, and the Helio runtime never sets it
	// (→ production).
	EnvEnvironment = "QUICKBOOKS_ENVIRONMENT"
)

// Service implements the built-in QuickBooks Online tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the API base (scheme+host, no trailing slash); empty
	// selects prod/sandbox from the environment. Tests point it at httptest.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one quickbooks subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, unknown
// subcommands) are exit 2; runtime/API errors (QBO non-2xx, transport failure)
// are exit 1. Errors render to stderr — JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	realm := env[EnvRealmID]
	// Absent credentials are a runtime/environment failure — the connection was
	// never injected — not a caller-fixable usage error. Render them as an
	// apiError so the emitted kind ("api" = the API/runtime category) agrees
	// with the exit 1 below; a usageError would emit kind "usage" (exit 2) and
	// disagree.
	if token == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: "QUICKBOOKS_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if realm == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: "QUICKBOOKS_REALM_ID is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	base := s.BaseURL
	if base == "" {
		base = baseURLFor(os.Getenv(EnvEnvironment))
	}
	cl := &client{base: base, realm: realm, token: token, hc: s.HC, out: s.stdout(), err: s.stderr()}

	root := s.newRoot(cl)
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
// format before cobra has parsed flags (the pre-parse credential checks).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP>,"fault":[…]}}; the
// fault array passes QBO's code/message/detail through (design §2).
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
		if len(apiErr.faults) > 0 {
			payload["fault"] = apiErr.faults
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

// newRoot builds the grouped-by-resource cobra tree. company/query/report are
// top-level; each accounting entity hangs under its own resource group.
func (s *Service) newRoot(cl *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "quickbooks",
		Short:         "QuickBooks Online built-in service (Accounting + Reports API v3)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(cl.out)
	root.SetErr(cl.err)
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(cl.newCompanyCmd(), cl.newQueryCmd(), cl.newReportCmd())
	for _, spec := range entities {
		root.AddCommand(cl.newEntityCmd(spec))
	}
	return root
}

// NewCommandTree returns the full command tree built with an empty client for
// dry-run parsing and traversal (tools.Service seam, design 318). Credentials
// are only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command {
	return s.newRoot(&client{out: s.stdout(), err: s.stderr()})
}
