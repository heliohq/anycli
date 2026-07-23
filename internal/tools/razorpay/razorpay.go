// Package razorpay is the built-in Razorpay service: a non-interactive cobra
// tree over the Razorpay REST surface (https://api.razorpay.com/v1). Auth is
// "Authorization: Bearer <access_token>" (the merchant's OAuth 2.0 access token
// minted by Razorpay's partner OAuth flow). It exposes read verbs across the
// gateway domain model — payments, orders, refunds, customers, payment links,
// settlements, and subscriptions — each as `list` (paginated) and `get <id>`.
//
// Razorpay errors are non-2xx with a JSON body carrying
// {"error":{"code","description",...}}; a 401 rejects the credential. Every
// command emits the provider JSON on stdout verbatim (passthrough + newline),
// including the {"entity":"collection","count":N,"items":[...]} list envelope.
// Amounts are in the smallest currency unit (paise for INR) and pass through
// unchanged; the tool never rescales.
//
// Exit codes: 0 success, 1 runtime/API failure (typed apiError), 2 usage/parse
// error. Under --json, errors render as a structured envelope on stderr.
package razorpay

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

// DefaultBaseURL is the production Razorpay API base. A few resources live on
// /v2; the per-resource paths pin their own version, so this is the /v1 root.
const DefaultBaseURL = "https://api.razorpay.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/razorpay.json). It carries the merchant's OAuth 2.0
// access token (Razorpay access tokens expire every 90 days; refresh is owned
// by integration-service, not this tool).
const EnvAccessToken = "RAZORPAY_ACCESS_TOKEN"

// sideEffectAnnotation is the design-318 approval-gate annotation key. Every
// runnable leaf in this tree is a read, so all carry the value "false".
const sideEffectAnnotation = "anycli.side_effect"

// Service implements the built-in Razorpay tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Razorpay API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// apiError is a runtime/API failure (Razorpay non-2xx, transport, or decode).
// status is the HTTP status when it originated from a response, else 0.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// usageError is a caller-input failure (missing/empty argument). It maps to
// exit code 2, distinct from apiError's exit code 1.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Execute runs one razorpay subcommand with the resolved credentials in env.
// Success is exit 0; runtime/API failures (apiError) are exit 1; usage/parse
// errors (usageError plus every cobra-originated parse/arg error) are exit 2.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "RAZORPAY_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "razorpay",
		Short:         "Razorpay built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	for _, r := range resources {
		root.AddCommand(s.newResourceCmd(token, r))
	}
	return root
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// missing-token check).
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
