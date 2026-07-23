// Package hootsuite is the built-in Hootsuite service: a non-interactive cobra
// tree over the Hootsuite REST API v1 (https://platform.hootsuite.com/v1). Auth
// is "Authorization: Bearer <token>" with a short-lived OAuth 2.0 user token.
// Every Hootsuite response wraps its payload in a top-level {"data": …}
// envelope; this service unwraps it and prints the inner value so agents see a
// provider-neutral result. Errors are non-2xx with a JSON body carrying an
// errors[] array of numeric code/message; 401 rejects the credential. Usage
// errors (bad flags, non-UTC send times) exit 2; API/runtime failures — including
// a never-injected credential — exit 1.
package hootsuite

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

// DefaultBaseURL is the production Hootsuite REST API v1 base.
const DefaultBaseURL = "https://platform.hootsuite.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/hootsuite.json). Hootsuite access tokens are short-lived
// (~1h) OAuth 2.0 bearer tokens refreshed by the Helio token gateway.
const EnvAccessToken = "HOOTSUITE_ACCESS_TOKEN"

// Service implements the built-in Hootsuite tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Hootsuite API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// usageError is a client-side input error (bad flag, illegal combo, non-UTC
// timestamp) → exit 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// runtimeError is a client-side runtime/environment precondition failure that is
// NOT the caller's usage mistake — e.g. the credential binding never injected
// HOOTSUITE_ACCESS_TOKEN. It exits 1 (like an API/transport failure), and its
// JSON envelope carries code (never the usage-error "invalid_request") with no
// HTTP status.
type runtimeError struct {
	code string
	msg  string
}

func (e *runtimeError) Error() string { return e.msg }

// Execute runs one hootsuite subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// A never-injected credential is a runtime precondition failure, not a
		// caller usage mistake → exit 1 with the runtime (not usage) envelope.
		// The check runs before cobra parses flags, so detect --json in the raw
		// args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &runtimeError{code: "unauthenticated", msg: "HOOTSUITE_ACCESS_TOKEN is not set"})
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
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
// {"error":{"code":…,"message":…,"status":<HTTP, omitted for usage errors>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	errObj := map[string]any{}
	var apiErr *apiError
	var useErr *usageError
	var runErr *runtimeError
	switch {
	case errors.As(err, &apiErr):
		code := apiErr.code
		if code == "" {
			code = "api_error"
		}
		errObj["code"] = code
		errObj["message"] = apiErr.message
		errObj["status"] = apiErr.status
	case errors.As(err, &runErr):
		code := runErr.code
		if code == "" {
			code = "runtime_error"
		}
		errObj["code"] = code
		errObj["message"] = runErr.Error()
	case errors.As(err, &useErr):
		errObj["code"] = "invalid_request"
		errObj["message"] = useErr.Error()
	default:
		errObj["code"] = "invalid_request"
		errObj["message"] = err.Error()
	}
	b, mErr := json.Marshal(map[string]any{"error": errObj})
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

// newRoot builds the resource-grouped cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "hootsuite",
		Short:         "Hootsuite built-in service (schedule and manage social posts)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	org := newGroupCmd("org", "Member organizations")
	org.AddCommand(s.newOrgListCmd(token))

	profile := newGroupCmd("profile", "Social profiles")
	profile.AddCommand(
		s.newProfileListCmd(token),
		s.newProfileGetCmd(token),
		s.newProfileTeamsCmd(token),
	)

	message := newGroupCmd("message", "Scheduled and queued posts")
	message.AddCommand(
		s.newMessageScheduleCmd(token),
		s.newMessageListCmd(token),
		s.newMessageGetCmd(token),
		s.newMessageDeleteCmd(token),
		s.newMessageApproveCmd(token),
		s.newMessageRejectCmd(token),
	)

	media := newGroupCmd("media", "Media uploads")
	media.AddCommand(
		s.newMediaCreateCmd(token),
		s.newMediaGetCmd(token),
	)

	root.AddCommand(s.newMeCmd(token), org, profile, message, media)
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
