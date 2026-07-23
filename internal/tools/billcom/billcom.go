// Package billcom is the built-in BILL (Bill.com) service: a cobra tree over
// the BILL Connect v3 REST API (https://developer.bill.com). BILL has no OAuth
// authorize flow — it authenticates with a developer key (devKey) plus a
// per-org credential login that mints a short-lived sessionId; both devKey and
// sessionId then ride as HTTP headers on every call. This service owns that
// login->call "session dance" per invocation (exactly what a service-type tool
// is for), so the Helio side needs no bespoke token machinery.
//
// Auth modes (BILLCOM_AUTH_MODE):
//   - "v3" (default): raw / Accountant-Console credential; login via
//     POST {v3base}/login with a JSON body.
//   - "sync_token": AP & AR sync token (the recommended, limited-access,
//     no-payments partner credential); login via the BILL v2 operation
//     POST {v2base}/api/v2/Login.json (form-encoded). The returned sessionId
//     still rides as a header on the v3 resource calls.
//
// Money-movement carve-out: payments are exposed read-only (list/get). Creating
// payments or bank accounts needs an elevated / MFA-trusted session that is out
// of scope, and is intrinsically unavailable on a sync-token session.
package billcom

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

// Env var names the credential bindings inject (definitions/tools/bill-com.json).
const (
	EnvDevKey   = "BILLCOM_DEV_KEY"
	EnvUsername = "BILLCOM_USERNAME"
	EnvPassword = "BILLCOM_PASSWORD"
	EnvOrgID    = "BILLCOM_ORG_ID"
	EnvAuthMode = "BILLCOM_AUTH_MODE"
	EnvEnv      = "BILLCOM_ENV"
	// EnvCredentials carries the whole credential set as one JSON object
	// {"dev_key","username","password","organization_id","auth_mode","env"}.
	// Helio projects the credential as discrete per-field env vars (the multi-
	// field manual_credentials_verified store → the individual BILLCOM_* vars
	// above), which the definition declares. This JSON blob is a convenience for
	// the dev harness / direct invocation; the discrete env vars take precedence
	// per field when set, so either shape works.
	EnvCredentials = "BILLCOM_CREDENTIALS"
)

// authModeSyncToken selects the v2-login path for the AP & AR sync token.
const authModeSyncToken = "sync_token"

// Service implements the built-in BILL tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the v3 gateway base (…/connect/v3); empty = derived
	// from BILLCOM_ENV. Tests point it at an httptest server.
	BaseURL string
	// LoginV2BaseURL overrides the v2 API base (host only, no path) used for
	// the sync-token login; empty = derived from BILLCOM_ENV.
	LoginV2BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one bill-com subcommand with the resolved credentials in env.
// Exit codes: 0 success; 2 usage/parse (bad flags, missing --data, unknown
// subcommand); 1 everything else (missing credentials, transport failure, BILL
// non-2xx via a typed apiError). Errors render to stderr — as a JSON envelope
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	c, cfgErr := s.newClient(env)
	if cfgErr != nil {
		// Config errors (missing required credential) surface before cobra
		// parses flags, so detect --json in the raw args for the envelope.
		s.renderError(hasJSONArg(args), cfgErr)
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(c)
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
	// usageError plus every cobra parse/arg/enum/unknown-command error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags.
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

// newRoot builds the resource-grouped cobra tree. The client carries the
// resolved credentials + per-invocation session state; its RunE closures do the
// login->call dance.
func (s *Service) newRoot(c *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "bill-com",
		Short:         "BILL (Bill.com) AP/AR built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (default for list/get/create)")

	root.AddCommand(
		s.newResourceGroup(c, "bill", "/bills", true),
		s.newResourceGroup(c, "vendor", "/vendors", true),
		s.newResourceGroup(c, "invoice", "/invoices", true),
		s.newResourceGroup(c, "customer", "/customers", true),
		// Payments are read-only (money-movement carve-out): no create verb.
		s.newResourceGroup(c, "payment", "/payments", false),
		s.newOrgCmd(c),
		s.newWhoamiCmd(c),
	)
	return root
}

// newGroupCmd is a runnable command group. Making the group runnable restores
// Args validation (cobra skips it on non-runnable commands, which would let an
// unknown subcommand exit 0 — a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with an empty client for
// dry-run parsing and traversal (tools.Service seam, design 318). The client is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(&client{}) }
