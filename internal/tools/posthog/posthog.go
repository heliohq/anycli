// Package posthog is the built-in PostHog service: a non-interactive cobra tree
// over the PostHog REST surface (https://us.posthog.com / https://eu.posthog.com
// under /api). Auth is "Authorization: Bearer <token>" and works identically for
// OAuth access tokens (pha_…) and personal API keys (phx_…). PostHog is
// region-split: the same token is valid in exactly one region, so the client
// resolves the region host per invocation (POSTHOG_API_HOST override, else probe
// US then EU) — see client.go. Every command emits the provider JSON on stdout
// (passthrough + newline); list commands pass through PostHog's
// {"count","next","previous","results"} envelope untouched.
package posthog

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

// USHost / EUHost are the two PostHog Cloud regions. Self-hosted instances are
// reached with POSTHOG_API_HOST instead (which disables the region probe).
const (
	USHost = "https://us.posthog.com"
	EUHost = "https://eu.posthog.com"
)

// EnvAccessToken / EnvAPIHost are the env vars the credential bindings inject
// (definitions/tools/posthog.json). The access token is required; the API host
// is optional (harness override, self-hosted, future metadata source).
const (
	EnvAccessToken = "POSTHOG_ACCESS_TOKEN"
	EnvAPIHost     = "POSTHOG_API_HOST"
)

// anycli.side_effect annotations (design 318): readOnly marks a leaf command
// that only reads provider state; writeAction marks one that mutates it. Note
// PostHog analytics query endpoints are reads even over HTTP POST.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in PostHog tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = region discovery. When set it
	// also disables the region probe (tests point it at an httptest server).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer

	// apiHost is POSTHOG_API_HOST, read once per Execute. When set it pins the
	// region host without a probe.
	apiHost string
	// region caches the resolved region host for the process lifetime, so at
	// most one probe happens per heliox invocation.
	region string
	// usHost / euHost override the region-probe targets in tests; empty means
	// the production USHost / EUHost constants.
	usHost string
	euHost string
}

// Execute runs one posthog subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (PostHog non-2xx, transport failure, region-resolution
// failure) are exit 1. Errors render to stderr — as JSON under --json, plain
// text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "POSTHOG_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	s.apiHost = env[EnvAPIHost]

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
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "posthog",
		Short:         "PostHog built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newWhoamiCmd(token),
		s.newProjectCmd(token),
		s.newQueryCmd(token),
		s.newInsightCmd(token),
		s.newDashboardCmd(token),
		s.newFlagCmd(token),
		s.newAnnotationCmd(token),
		s.newPersonCmd(token),
		s.newCohortCmd(token),
		s.newExperimentCmd(token),
		s.newRecordingCmd(token),
		s.newEventDefinitionCmd(token),
		s.newPropertyDefinitionCmd(token),
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
	return strings.TrimRight(s.BaseURL, "/")
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
