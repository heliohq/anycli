// Package segment is the built-in Twilio Segment service: a read-first,
// non-interactive cobra tree over the Segment Public API
// (https://api.segmentapis.com). Auth is a workspace-scoped Public API token
// sent as "Authorization: Bearer <token>". The tool is management/observability
// only — the Tracking API (event ingest, per-source write keys) is deliberately
// out of scope.
//
// Output is provider-passthrough: every command emits the provider's JSON body
// on stdout verbatim (data + full pagination block, including previous), so an
// agent can page with --cursor without the tool reshaping Segment's envelope.
// Errors are exit 1 (runtime/API) or exit 2 (usage/parse); under --json they
// render as {"error":{"message":…,"kind":"usage|api","status":<HTTP>}}.
package segment

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

// DefaultBaseURL is the production Segment Public API base for US-resident
// workspaces. v1 is US-scoped (see DESIGN §1 data residency); an EU workspace
// token verifies against eu1.api.segmentapis.com, not this host.
const DefaultBaseURL = "https://api.segmentapis.com"

// EnvToken is the env var the credential binding injects
// (definitions/tools/segment.json). Segment Public API tokens are
// workspace-scoped and long-lived (no expiry, no refresh).
const EnvToken = "SEGMENT_TOKEN"

// Service implements the built-in Segment tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Segment API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one segment subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad values,
// unknown subcommands) are exit 2; runtime/API errors (Segment non-2xx,
// transport failure) are exit 1. A 401 additionally rejects the credential so
// the token gateway refresh path (design 227) triggers.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "SEGMENT_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. Every leaf is read-only;
// writes are reachable only through the raw `request` verb with an explicit
// non-GET --method.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "segment",
		Short:         "Twilio Segment Public API (management & observability)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "structured JSON error envelope on failure (output is always JSON)")

	workspace := newGroupCmd("workspace", "The current workspace")
	workspace.AddCommand(s.newWorkspaceGetCmd(token))

	source := newGroupCmd("source", "Sources (data inputs)")
	source.AddCommand(
		s.newSourceListCmd(token),
		s.newSourceGetCmd(token),
		s.newSourceConnectedDestinationsCmd(token),
	)

	destination := newGroupCmd("destination", "Destinations (data outputs)")
	destination.AddCommand(
		s.newDestinationListCmd(token),
		s.newDestinationGetCmd(token),
	)

	warehouse := newGroupCmd("warehouse", "Warehouses")
	warehouse.AddCommand(
		s.newWarehouseListCmd(token),
		s.newWarehouseGetCmd(token),
	)

	trackingPlan := newGroupCmd("tracking-plan", "Tracking plans (governance)")
	trackingPlan.AddCommand(
		s.newTrackingPlanListCmd(token),
		s.newTrackingPlanGetCmd(token),
		s.newTrackingPlanRulesCmd(token),
	)

	function := newGroupCmd("function", "Functions")
	function.AddCommand(s.newFunctionListCmd(token))

	space := newGroupCmd("space", "Unify spaces")
	space.AddCommand(
		s.newSpaceListCmd(token),
		s.newSpaceAudiencesCmd(token),
	)

	iam := newGroupCmd("iam", "Access management (users & groups)")
	iamUser := newGroupCmd("user", "IAM users")
	iamUser.AddCommand(s.newIAMUserListCmd(token))
	iamGroup := newGroupCmd("group", "IAM user groups")
	iamGroup.AddCommand(s.newIAMGroupListCmd(token))
	iam.AddCommand(iamUser, iamGroup)

	delivery := newGroupCmd("delivery", "Delivery & event-volume observability")
	delivery.AddCommand(
		s.newEventsVolumeCmd(token),
		s.newDeliveryMetricsCmd(token),
	)

	root.AddCommand(
		workspace, source, destination, warehouse, trackingPlan,
		function, space, iam, delivery,
		s.newRequestCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// classifyCredentialError marks a 401 (or an unauthorized error code) as an
// explicit credential rejection so the token gateway refresh path triggers; any
// other status is a plain runtime error.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// isUnauthorizedBody reports whether a Segment error body's first error type is
// an unauthorized/authentication signal (defensive: some 401s arrive with the
// type in the body, mirrored from the notion precedent).
func isUnauthorizedBody(body []byte) bool {
	var env struct {
		Errors []struct {
			Type string `json:"type"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Errors) == 0 {
		return false
	}
	t := strings.ToLower(env.Errors[0].Type)
	return strings.Contains(t, "unauthorized") || strings.Contains(t, "authentication")
}
