// Package klaviyo is the built-in Klaviyo service: a non-interactive cobra tree
// over the Klaviyo JSON:API surface (https://a.klaviyo.com/api). It covers the
// audience, messaging, and analytics jobs an AI teammate does with Klaviyo —
// profiles, lists, segments, campaigns, flows, metrics, events, templates, and
// reporting.
//
// Auth is an OAuth 2.0 bearer access token injected as KLAVIYO_ACCESS_TOKEN; a
// Klaviyo private API key (documented "pk_" prefix) is also accepted and sent
// with the Klaviyo-API-Key scheme so the dev harness can run before the OAuth
// app exists. Every request carries the pinned `revision` header. Klaviyo
// errors are a JSON:API `errors` array on a non-2xx status; a 401 rejects the
// credential.
//
// Output: each command emits the provider JSON:API response on stdout verbatim
// (+ newline). --json is accepted for uniformity and switches error rendering
// to a structured envelope on stderr.
package klaviyo

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

// design-318 side_effect annotation maps shared by every runnable leaf.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// DefaultBaseURL is the production Klaviyo API base (token/data host, not www).
const DefaultBaseURL = "https://a.klaviyo.com/api"

// apiRevision is the dated API version sent on every request. Klaviyo marks the
// `revision` header required on each endpoint; this is the single owner.
const apiRevision = "2026-07-15"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/klaviyo.json).
const EnvAccessToken = "KLAVIYO_ACCESS_TOKEN"

// Service implements the built-in Klaviyo tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Klaviyo API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one klaviyo subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flag combos, bad enums, invalid
// JSON, missing required flags, unknown subcommands) are exit 2; runtime/API
// errors (Klaviyo non-2xx, transport failure, decode failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "KLAVIYO_ACCESS_TOKEN is not set"})
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
		return execution.Failure(err), nil
	}
	return execution.Result{ExitCode: 2}, nil
}

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "klaviyo",
		Short:         "Klaviyo built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	root.AddCommand(
		s.newAccountCmd(token),
		s.newProfileCmd(token),
		s.newListCmd(token),
		s.newSegmentCmd(token),
		s.newCampaignCmd(token),
		s.newFlowCmd(token),
		s.newMetricCmd(token),
		s.newEventCmd(token),
		s.newTemplateCmd(token),
		s.newReportCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group prints help, but an
// unknown subcommand still fails (cobra skips Args validation on non-runnable
// commands, which would return exit 0 for an unknown subcommand — a false
// success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
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
