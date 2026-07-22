// Package adyen is the built-in Adyen service: a non-interactive cobra tree
// over the Adyen Management API v3 (https://management-live.adyen.com/v3).
//
// v1 is Management-only and live-only (design 008-300 catalog row 170; the
// per-tool DESIGN on branch tool/adyen). It exposes read/config introspection
// — API-credential identity, merchant/company config, payment-method settings,
// webhooks, stores, terminals — and moves no money. The Checkout surface
// (payment links + refund/capture/cancel) is a documented v2 follow-up.
//
// Auth is a raw "X-API-Key: <key>" header (never Bearer). Per Adyen's error
// docs a 401 is an authentication failure (the key is missing/incorrect) and
// rejects the credential; a role/permission 403 (errorCode 010) authenticated
// successfully but lacks the endpoint role — self-service-fixable in the
// Customer Area — so it passes through as an ordinary API error, never a
// rejection. Every command emits the provider JSON on stdout verbatim.
package adyen

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

// DefaultBaseURL is the production Adyen Management API v3 base. It is LIVE:
// the Helio connect verifier is live-fixed, so a connected credential is always
// a live key, and the connected surface targets the live host. The Adyen test
// host (management-test.adyen.com/v3) is reached only via the anycli L2 harness
// overriding Service.BaseURL — never through a heliox-exposed flag.
const DefaultBaseURL = "https://management-live.adyen.com/v3"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/adyen.json). Adyen API keys are long-lived and
// non-expiring (rotated/revoked manually in the Customer Area).
const EnvAPIKey = "ADYEN_API_KEY"

// Service implements the built-in Adyen tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Management API base; empty = DefaultBaseURL. Tests
	// and the L2 harness point it at an httptest server or management-test.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one adyen subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, missing required
// scope, unknown subcommands) are exit 2; runtime/API errors (Adyen non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "ADYEN_API_KEY is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "adyen",
		Short:         "Adyen built-in service (Management API v3, X-API-Key)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(s.newManagementCmd(key))
	return root
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// missing-key check).
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
// "errorCode":<Adyen code or omitted>}}.
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
		if apiErr.errorCode != "" {
			payload["errorCode"] = apiErr.errorCode
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
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

// NewCommandTree returns the full command tree built with an empty key for
// dry-run parsing and traversal (tools.Service seam, design 318). The key is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
