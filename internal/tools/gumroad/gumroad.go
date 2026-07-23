// Package gumroad is the built-in Gumroad service: a non-interactive cobra
// tree over the Gumroad API v2 surface (https://api.gumroad.com/v2). Auth is
// "Authorization: Bearer <token>". Gumroad speaks a success-envelope dialect:
// every response carries a boolean "success" field, and a request can fail
// with HTTP 200 but "success":false (e.g. a not-found product), so the status
// code alone is not the success signal — the service reads the success field.
// A 401 rejects the credential; other non-success responses are runtime errors.
package gumroad

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

// DefaultBaseURL is the production Gumroad API v2 base. Note: only the /v2 REST
// resources live on api.gumroad.com; the OAuth authorize/token endpoints live
// on gumroad.com and are the host's concern, not this tool's.
const DefaultBaseURL = "https://api.gumroad.com/v2"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/gumroad.json). Gumroad OAuth access tokens are
// non-expiring.
const EnvAccessToken = "GUMROAD_ACCESS_TOKEN"

// Service implements the built-in Gumroad tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Gumroad API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one gumroad subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Gumroad success:false, non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &apiError{message: "GUMROAD_ACCESS_TOKEN is not set"})
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
	// Every cobra-originated parse/arg/enum/unknown-command error is a usage
	// error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "gumroad",
		Short:         "Gumroad built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newUserCmd(token),
		s.newProductCmd(token),
		s.newSaleCmd(token),
		s.newSubscriberCmd(token),
		s.newOfferCodeCmd(token),
		s.newLicenseCmd(token),
	)
	return root
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
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
		// Carry the provider's own message; the HTTP status is a separate
		// field, so the "(HTTP nnn)" prefix in Error() is redundant here.
		payload["message"] = apiErr.message
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
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
