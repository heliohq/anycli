// Package onesignal is the built-in OneSignal service: a non-interactive cobra
// tree over the OneSignal REST API (https://api.onesignal.com). Unlike a
// single-token tool, OneSignal is scoped by two injected credentials — an App
// API Key (the secret, sent as "Authorization: Key <key>", note the scheme word
// is "Key", not "Bearer" or "Basic") and an App ID (a public UUID that scopes
// every request). The App ID is auto-injected into each request's body, query,
// or path so the agent never passes it. Every command emits the provider JSON
// on stdout (passthrough + newline). Usage/parse errors are exit 2; runtime/API
// errors are exit 1 (a 401 additionally rejects the credential).
package onesignal

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

// DefaultBaseURL is the current OneSignal REST API base. The legacy
// https://onesignal.com/api/v1 host still resolves, but the service targets the
// current host.
const DefaultBaseURL = "https://api.onesignal.com"

// EnvAppAPIKey and EnvAppID are the env vars the credential bindings inject
// (definitions/tools/onesignal.json). The App API Key is the per-app secret;
// the App ID is a public UUID scoping every call. Both are non-expiring until
// the user rotates them in the OneSignal dashboard.
const (
	EnvAppAPIKey = "ONESIGNAL_APP_API_KEY"
	EnvAppID     = "ONESIGNAL_APP_ID"
)

// Service implements the built-in OneSignal tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle). The
// registered instance is shared across invocations, so per-invocation
// credentials are threaded through newRoot, never stored on the struct.
type Service struct {
	// BaseURL overrides the OneSignal API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one onesignal subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands, and the
// "exactly one targeting method" rule) are exit 2; runtime/API errors
// (OneSignal non-2xx, transport failure) are exit 1. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAppAPIKey]
	appID := env[EnvAppID]
	if key == "" || appID == "" {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		missing := EnvAppAPIKey
		if key != "" {
			missing = EnvAppID
		}
		s.renderError(hasJSONArg(args), &usageError{msg: missing + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key, appID)
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

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-credential check).
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

// newRoot builds the resource-grouped cobra tree. Every command captures the
// per-invocation App API Key (auth) and App ID (request scope).
func (s *Service) newRoot(key, appID string) *cobra.Command {
	root := &cobra.Command{
		Use:           "onesignal",
		Short:         "OneSignal built-in service (push / email / SMS messaging)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	message := newGroupCmd("message", "Send and inspect messages")
	message.AddCommand(
		s.newMessageSendCmd(key, appID),
		s.newMessageListCmd(key, appID),
		s.newMessageGetCmd(key, appID),
		s.newMessageCancelCmd(key, appID),
	)
	segment := newGroupCmd("segment", "Manage audience segments")
	segment.AddCommand(
		s.newSegmentCreateCmd(key, appID),
		s.newSegmentListCmd(key, appID),
		s.newSegmentDeleteCmd(key, appID),
	)
	user := newGroupCmd("user", "Create and read users by alias")
	user.AddCommand(
		s.newUserUpsertCmd(key, appID),
		s.newUserGetCmd(key, appID),
	)

	root.AddCommand(message, segment, user)
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
