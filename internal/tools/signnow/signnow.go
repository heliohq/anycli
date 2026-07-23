// Package signnow is the built-in SignNow service: a non-interactive cobra
// tree over the SignNow REST surface (https://api.signnow.com). It sends
// documents out for signature, tracks field invites, and downloads executed
// PDFs on behalf of an AI teammate.
//
// Auth is "Authorization: Bearer <token>" (an OAuth 2.0 access token minted by
// Helio's token gateway). The API base defaults to production and can be
// pointed at the free eval sandbox (https://api-eval.signnow.com) through the
// SIGNNOW_API_BASE_URL env var — an optional credential binding the token
// gateway never sets, so production callers always hit production.
//
// SignNow errors are non-2xx with one of two JSON dialects — the current
// {"errors":[{"code","message"}]} and the legacy {"error":"..."} — and a 401
// rejects the credential. The exit-code contract is 0 success, 1 runtime/API
// failure, 2 usage/parse error; under --json errors render as a structured
// envelope.
package signnow

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

// DefaultBaseURL is the production SignNow API base.
const DefaultBaseURL = "https://api.signnow.com"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/signnow.json).
const EnvAccessToken = "SIGNNOW_ACCESS_TOKEN"

// EnvBaseURL is the optional PROCESS env var that overrides the API base
// (sandbox targeting for the L2 dev harness). It is deliberately NOT a
// credential binding: the base URL is a fixed production constant, not a
// token-gateway-served credential, so projecting it through the connection
// would be a false contract (and the helio-cli pin-match check requires every
// anycli credential field to be projected by the bundle). The dev harness sets
// it as an ordinary env var; production never sets it, so the service falls
// back to DefaultBaseURL.
const EnvBaseURL = "SIGNNOW_API_BASE_URL"

// Service implements the built-in SignNow tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the SignNow API base; empty = DefaultBaseURL (or the
	// SIGNNOW_API_BASE_URL env value, resolved per invocation). Tests point it
	// at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one signnow subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, invalid JSON,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (SignNow non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	// Resolve the base URL per invocation without mutating the shared singleton
	// registered in register.go: BaseURL set on the struct (tests) wins, then
	// the process-env override (dev harness only), then the production default.
	inv := *s
	if inv.BaseURL == "" {
		inv.BaseURL = os.Getenv(EnvBaseURL)
	}

	root := inv.newRoot(token)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	inv.renderError(jsonMode, err)

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

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (GET list/get/download), "true" for
// provider-state mutations (upload / add-fields / delete / invite / template /
// link create).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// newRoot builds the grouped-by-resource cobra tree: whoami is top-level;
// document / invite / template / link each hang under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "signnow",
		Short:         "SignNow built-in service (e-signature)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	document := newGroupCmd("document", "Upload, list, inspect, and download documents")
	document.AddCommand(
		s.newDocumentListCmd(token),
		s.newDocumentGetCmd(token),
		s.newDocumentUploadCmd(token),
		s.newDocumentAddFieldsCmd(token),
		s.newDocumentDownloadCmd(token),
		s.newDocumentDeleteCmd(token),
	)
	invite := newGroupCmd("invite", "Send, resend, and cancel signature invites")
	invite.AddCommand(
		s.newInviteSendCmd(token),
		s.newInviteResendCmd(token),
		s.newInviteCancelCmd(token),
	)
	template := newGroupCmd("template", "Create and instantiate reusable templates")
	template.AddCommand(
		s.newTemplateCreateCmd(token),
		s.newTemplateCopyCmd(token),
	)
	link := newGroupCmd("link", "Create signing links")
	link.AddCommand(
		s.newLinkCreateCmd(token),
	)

	root.AddCommand(
		s.newWhoamiCmd(token),
		document, invite, template, link,
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
