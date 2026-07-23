// Package courier is the built-in Courier service: a non-interactive cobra tree
// over the Courier REST surface (https://api.courier.com). Courier is
// notification infrastructure — the load-bearing verb is `send` (dispatch a
// notification to a recipient across the workspace's configured channels);
// everything else reads, tracks, or discovers send targets around it. Auth is
// "Authorization: Bearer <api_key>". Courier fails with a non-2xx status and a
// JSON body carrying a message; 401 rejects the credential. Every command emits
// the provider JSON on stdout verbatim (passthrough + newline).
package courier

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

// DefaultBaseURL is the production Courier API base.
const DefaultBaseURL = "https://api.courier.com"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/courier.json). Courier keys are non-expiring,
// workspace-scoped Bearer tokens.
const EnvAPIKey = "COURIER_API_KEY"

// Service implements the built-in Courier tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Courier API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one courier subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, missing required
// flags, invalid JSON, unknown subcommands) are exit 2; runtime/API errors
// (Courier non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "COURIER_API_KEY is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. send is top-level (the
// core action); everything else hangs under a resource group.
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "courier",
		Short:         "Courier built-in service (notification infrastructure)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	message := newGroupCmd("message", "Track and manage sent messages")
	message.AddCommand(
		s.newMessageGetCmd(key),
		s.newMessageListCmd(key),
		s.newMessageHistoryCmd(key),
		s.newMessageCancelCmd(key),
	)
	list := newGroupCmd("list", "Discover and manage mailing lists")
	list.AddCommand(
		s.newListGetCmd(key),
		s.newListListCmd(key),
		s.newListSubscribeCmd(key),
		s.newListUnsubscribeCmd(key),
	)
	audience := newGroupCmd("audience", "Discover audiences")
	audience.AddCommand(
		s.newAudienceGetCmd(key),
		s.newAudienceListCmd(key),
	)
	profile := newGroupCmd("profile", "Read recipient profiles")
	profile.AddCommand(
		s.newProfileGetCmd(key),
		s.newProfileSubscriptionsCmd(key),
	)
	brand := newGroupCmd("brand", "Resolve brands for branded sends")
	brand.AddCommand(
		s.newBrandGetCmd(key),
		s.newBrandListCmd(key),
	)
	automation := newGroupCmd("automation", "Trigger ad-hoc automations")
	automation.AddCommand(s.newAutomationInvokeCmd(key))

	root.AddCommand(
		s.newSendCmd(key),
		message, list, audience, profile, brand, automation,
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
