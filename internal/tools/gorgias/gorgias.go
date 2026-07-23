// Package gorgias is the built-in Gorgias service: a non-interactive cobra tree
// over the per-account Gorgias REST surface. Every Gorgias helpdesk is keyed by
// its subdomain (acme.gorgias.com) and ALL REST traffic goes to that host, so
// the base URL is built from the injected GORGIAS_SUBDOMAIN
// (https://{subdomain}.gorgias.com/api) and auth is
// "Authorization: Bearer <access_token>". Gorgias errors are non-2xx with a JSON
// body carrying an "error" attribute; a 401 rejects the credential. List
// endpoints are cursor-paginated ({"data":[...],"meta":{"next_cursor":...}}) and
// every command emits the provider JSON on stdout verbatim.
package gorgias

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

// EnvAccessToken is the env var the credential binding injects with the OAuth2
// access token (definitions/tools/gorgias.json).
const EnvAccessToken = "GORGIAS_ACCESS_TOKEN"

// EnvSubdomain is the env var the credential binding injects with the account
// subdomain used to build the per-account base URL.
const EnvSubdomain = "GORGIAS_SUBDOMAIN"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for state-free reads, "true" for provider
// mutations. Group commands must not carry either.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Gorgias tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the derived per-account base URL; empty = built from the
	// injected subdomain. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one gorgias subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required flags,
// unknown subcommands) are exit 2; runtime/API errors (Gorgias non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "GORGIAS_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.resolveBaseURL(s.BaseURL, env[EnvSubdomain])
	if base == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "GORGIAS_SUBDOMAIN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, base)
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

// resolveBaseURL returns the explicit override (trailing-slash trimmed) when set,
// otherwise builds the per-account base from the subdomain. An empty subdomain
// with no override yields "" so the caller can fail fast.
func (s *Service) resolveBaseURL(override, subdomain string) string {
	if override != "" {
		return strings.TrimRight(override, "/")
	}
	if subdomain == "" {
		return ""
	}
	return "https://" + subdomain + ".gorgias.com/api"
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token, base string) *cobra.Command {
	root := &cobra.Command{
		Use:           "gorgias",
		Short:         "Gorgias helpdesk built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newTicketCmd(token, base),
		s.newMessageCmd(token, base),
		s.newCustomerCmd(token, base),
		s.newUserCmd(token, base),
		s.newTagCmd(token, base),
		s.newViewCmd(token, base),
		s.newSatisfactionCmd(token, base),
		s.newAccountCmd(token, base),
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group shows help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, which
// would be a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
