// Package novu is the built-in Novu service: a non-interactive cobra tree over
// Novu's REST API (https://api.novu.co, EU https://eu.api.novu.co). Novu is
// notification infrastructure — one `event trigger` fans a workflow out to a
// subscriber or topic across the channels the workflow defines. Auth is the
// literal "Authorization: ApiKey <secret>" scheme (NOT Bearer); the environment
// secret key is region-scoped (US/EU) and per-environment.
//
// The API surface is versioned per-resource: events, messages, notifications,
// integrations, and environments live under /v1; subscribers, topics, and
// workflows live under /v2 (Novu moved their CRUD there). The injected base URL
// is the region host only (no version), and each command builds its own
// versioned path. Every command emits the provider JSON on stdout verbatim, so
// nothing — including a trigger's status/error outcome fields — is dropped.
package novu

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

// DefaultBaseURL is the US-region Novu API host (no version suffix; commands add
// /v1 or /v2 per endpoint). The EU host is https://eu.api.novu.co.
const DefaultBaseURL = "https://api.novu.co"

// EnvSecretKey / EnvAPIBase are the env vars the credential bindings inject
// (definitions/tools/novu.json). EnvAPIBase is optional; empty = DefaultBaseURL.
const (
	EnvSecretKey = "NOVU_SECRET_KEY"
	EnvAPIBase   = "NOVU_API_BASE"
)

// Service implements the built-in Novu tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Novu API host; empty = EnvAPIBase, then
	// DefaultBaseURL. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one novu subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Novu
// non-2xx, transport failure) are exit 1 via the typed apiError. Errors render
// to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	secret := env[EnvSecretKey]
	if secret == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvSecretKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.BaseURL
	if base == "" {
		base = env[EnvAPIBase]
	}
	root := s.newRoot(secret, base)
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
// pick the error format before cobra has parsed flags (the pre-parse
// missing-secret check).
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

// client returns the configured HTTP client bound to the resolved base host and
// secret; every command builds requests through it.
func (s *Service) newClient(secret, base string) *client {
	host := base
	if host == "" {
		host = DefaultBaseURL
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	return &client{host: strings.TrimRight(host, "/"), secret: secret, hc: hc}
}

// newRoot builds the resource-grouped cobra tree.
func (s *Service) newRoot(secret, base string) *cobra.Command {
	root := &cobra.Command{
		Use:           "novu",
		Short:         "Novu notification infrastructure built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	c := s.newClient(secret, base)
	root.AddCommand(
		s.newEventCmd(c),
		s.newSubscriberCmd(c),
		s.newTopicCmd(c),
		s.newWorkflowCmd(c),
		s.newMessageCmd(c),
		s.newActivityCmd(c),
		s.newIntegrationCmd(c),
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group shows help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, a
// false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
