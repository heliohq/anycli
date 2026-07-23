// Package freshservice is the built-in Freshservice service: a non-interactive
// cobra tree over the Freshservice v2 REST surface
// (https://<domain>.freshservice.com/api/v2). It is IT Service Management
// (ITSM): tickets (incidents + service requests), their conversations,
// requesters, agents, groups, and CMDB assets.
//
// Auth is per-account. Helio persists exactly one secret per connection, so
// the credential is a single URL-shaped blob —
// https://<api_key>@<domain>.freshservice.com — carrying both the account
// domain and the API key. The service parses it once: the host builds the
// base URL (https://<domain>.freshservice.com/api/v2) and the userinfo
// username is the API key, sent as HTTP Basic auth with a dummy password
// (Authorization: Basic base64(<api_key>:X), per the official docs). The
// api_key never appears in a request URL — only in the Authorization header.
//
// Exit codes: 0 success · 1 runtime/API failure (typed apiError, HTTP status +
// provider body) · 2 usage/parse errors and a missing/malformed credential.
// Errors render to stderr — JSON under --json, plain text otherwise.
package freshservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// readOnly / writeAction carry the design-318 side-effect annotation for runnable leaves.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// EnvURL is the env var the credential binding injects
// (definitions/tools/freshservice.json). The resolved value is the URL blob
// https://<api_key>@<domain>.freshservice.com.
const EnvURL = "FRESHSERVICE_URL"

// apiVersionPath is appended to the account host to build the API base.
const apiVersionPath = "/api/v2"

// Service implements the built-in Freshservice tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the derived API base; empty = https://<host>/api/v2
	// from the credential blob. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// credential is the parsed FRESHSERVICE_URL blob: the API key (from userinfo)
// and the account host (e.g. acme.freshservice.com).
type credential struct {
	apiKey string
	host   string
}

// parseCredential splits the URL blob into its API key and account host. Both
// must be present; anything else is a usage error that never echoes the secret.
func parseCredential(blob string) (credential, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return credential{}, &usageError{msg: EnvURL + " is not set"}
	}
	u, err := url.Parse(blob)
	if err != nil {
		return credential{}, &usageError{msg: EnvURL + " is not a valid URL (expected https://<api_key>@<domain>.freshservice.com)"}
	}
	if u.User == nil || u.User.Username() == "" {
		return credential{}, &usageError{msg: EnvURL + " is missing the API key in userinfo (expected https://<api_key>@<domain>.freshservice.com)"}
	}
	if u.Host == "" {
		return credential{}, &usageError{msg: EnvURL + " is missing the account domain (expected https://<api_key>@<domain>.freshservice.com)"}
	}
	return credential{apiKey: u.User.Username(), host: u.Host}, nil
}

// Execute runs one freshservice subcommand with the resolved credential in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	cred, err := parseCredential(env[EnvURL])
	if err != nil {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 2}, nil
	}

	c := &client{
		baseURL: s.resolveBaseURL(cred.host),
		apiKey:  cred.apiKey,
		hc:      s.httpClient(),
	}
	root := s.newRoot(c)
	root.SetArgs(args)
	execErr := root.ExecuteContext(ctx)
	if execErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, execErr)

	var apiErr *apiError
	if errors.As(execErr, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(execErr), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// resolveBaseURL returns the test override when set, else the per-account base
// https://<host>/api/v2 derived from the credential blob.
func (s *Service) resolveBaseURL(host string) string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return "https://" + host + apiVersionPath
}

func (s *Service) httpClient() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// newRoot builds the grouped-by-resource cobra tree. Every resource hangs under
// its own group (ticket, requester, agent, group, asset).
func (s *Service) newRoot(c *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "freshservice",
		Short:         "Freshservice ITSM built-in service (tickets, requesters, agents, groups, assets)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (always on; accepted for uniformity)")

	root.AddCommand(
		s.newTicketCmd(c),
		s.newRequesterCmd(c),
		s.newAgentCmd(c),
		s.newGroupCmd(c),
		s.newAssetCmd(c),
	)
	return root
}

// newResourceGroup is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newResourceGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"status":<http>,"message":…,"provider_code":…,"retry_after":…}}.
// status/provider_code/retry_after are present only for API errors that carry
// them.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if apiErr.providerCode != "" {
			payload["provider_code"] = apiErr.providerCode
		}
		if apiErr.retryAfter != "" {
			payload["retry_after"] = apiErr.retryAfter
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

// usageError is a parse/validation error that maps to exit code 2. It never
// carries provider data.
type usageError struct {
	msg string
}

func (e *usageError) Error() string { return e.msg }
