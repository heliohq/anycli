// Package lemonsqueezy is the built-in Lemon Squeezy service: a non-interactive
// cobra tree over the Lemon Squeezy v1 REST surface
// (https://api.lemonsqueezy.com/v1). The API is JSON:API
// (https://jsonapi.org) and REQUIRES the vnd.api+json media type on every
// request; auth is "Authorization: Bearer <api_key>". A generated key is valid
// for ~1 year with no refresh cycle (design row #173, api_key lane).
//
// Output is the provider's JSON:API document passed through verbatim on stdout
// (the {data, meta, links, included} envelope); errors surface Lemon Squeezy's
// {"errors":[{status,title,detail}]} body. Exit codes follow the shared
// service contract: 0 success, 1 runtime/API failure, 2 usage/parse error.
package lemonsqueezy

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

// DefaultBaseURL is the production Lemon Squeezy v1 API base (the major version
// is a path prefix on every endpoint).
const DefaultBaseURL = "https://api.lemonsqueezy.com/v1"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/lemon-squeezy.json). The resolved value is a Lemon
// Squeezy API key (live or test mode); it rides the Authorization header with
// the Bearer scheme.
const EnvAPIKey = "LEMONSQUEEZY_API_KEY"

// JSON:API media type. Lemon Squeezy answers plain application/json requests
// with 406 Not Acceptable, so both headers are load-bearing on every call.
const mediaType = "application/vnd.api+json"

// Service implements the built-in Lemon Squeezy tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it
	// at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one lemon-squeezy subcommand with the resolved API key in env.
// Success is exit 0; usage/param errors (bad flag value, missing required
// flag, invalid JSON, unknown subcommand) are exit 2; runtime/API errors
// (Lemon Squeezy non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIKey]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		jsonMode, _ := root.PersistentFlags().GetBool("json")
		s.renderError(jsonMode, err)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return execution.Failure(err), nil
		}
		// usageError plus every cobra-originated parse/arg/unknown-command
		// error is inherently a usage error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
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

// renderError writes err to stderr. Under --json the shape mirrors the notion
// reference envelope: {"error":{"message":…,"kind":"usage|api","status":<HTTP
// or omitted>}}.
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

// newRoot builds the grouped-by-resource cobra tree mirroring the JSON:API
// resource set. whoami is top-level (cross-resource identity); everything else
// hangs under its resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "lemon-squeezy",
		Short:         "Lemon Squeezy built-in service (JSON:API, API key)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	whoami := &cobra.Command{
		Use:         "whoami",
		Short:       "Show the authenticated user (GET /users/me)",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd.Context(), token, "/users/me", nil)
		},
	}

	root.AddCommand(whoami)
	root.AddCommand(s.resourceGroups(token)...)
	return root
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }

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
