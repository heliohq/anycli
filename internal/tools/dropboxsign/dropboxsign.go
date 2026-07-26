// Package dropboxsign is the built-in Dropbox Sign (formerly HelloSign)
// service: a non-interactive cobra tree over the Dropbox Sign v3 REST surface
// (https://api.hellosign.com/v3). It drives the send -> track -> download loop
// an AI teammate actually uses — send a document for signature (uploaded file
// or remote URL, or a saved template), list and inspect requests, remind or
// cancel signers, and pull the completed signed PDF back — plus template
// listing.
//
// Auth is "Authorization: Bearer <access_token>" (the user's OAuth token,
// injected as DROPBOX_SIGN_ACCESS_TOKEN). Dropbox Sign fails with a non-2xx
// status and a JSON body of the shape {"error":{"error_msg","error_name"}};
// every call surfaces both. Exit codes follow the built-in service contract:
// 0 success, 2 usage/parse errors, 1 runtime/API failure (a typed apiError,
// with a 401 credential rejection classified for the resolver).
package dropboxsign

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

// readOnly / writeAction carry the design-318 side-effect annotation for runnable leaves.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// DefaultBaseURL is the production Dropbox Sign v3 API base. The legacy
// HelloSign host is retained by the provider (the product is Dropbox Sign; the
// API still lives under hellosign.com).
const DefaultBaseURL = "https://api.hellosign.com/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/dropbox-sign.json). It carries the user's OAuth 2.0
// access token, sent as a Bearer credential on every call.
const EnvAccessToken = "DROPBOX_SIGN_ACCESS_TOKEN"

// Service implements the built-in Dropbox Sign tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it at
	// an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one dropbox-sign subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (bad flags, missing required
// flags, unknown subcommands) are exit 2; runtime/API errors (non-2xx,
// transport failure) are exit 1. Errors render to stderr — JSON under --json,
// plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "DROPBOX_SIGN_ACCESS_TOKEN is not set"})
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
	// inherently a usage error -> exit 2.
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "dropbox-sign",
		Short: "Dropbox Sign (HelloSign) built-in service",
		Long: "Wraps the Dropbox Sign v3 API as the connected account. Dropbox Sign was\n" +
			"formerly HelloSign, which is why the API still lives on hellosign.com; it is\n" +
			"a DIFFERENT product from Dropbox file storage and cannot browse or read files\n" +
			"there.\n" +
			"\n" +
			"The loop is send -> track -> download. `signature-request send` (or\n" +
			"`send-with-template`) returns a `signature_request_id`; `signature-request\n" +
			"get` reports per-signer progress on it; `signature-request files` pulls the\n" +
			"document back. `template` is read-only here — templates are authored in the\n" +
			"Dropbox Sign web app and this tool only lists and sends with them.\n" +
			"\n" +
			"`--test-mode` on both send commands produces a watermarked, non-binding\n" +
			"request that neither consumes signature quota nor requires a paid plan. A\n" +
			"real send from a free plan is rejected with 402, so rehearse with it.\n" +
			"\n" +
			"A non-test send emails real people and creates a legally binding document the\n" +
			"moment the call returns. `signature-request cancel` stops a request that is\n" +
			"still incomplete; nothing reverses a completed signature.\n" +
			"\n" +
			"Both list verbs page with `--page` (1-based) and `--page-size`, and take\n" +
			"`--query` in Dropbox Sign's own search syntax rather than a bare substring.\n" +
			"Leaving the paging flags unset sends no paging parameters at all, so the\n" +
			"provider default page applies.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	sigReq := newGroupCmd("signature-request", "Send, track, and download signature requests")
	sigReq.AddCommand(
		s.newSendCmd(token),
		s.newSendWithTemplateCmd(token),
		s.newListCmd(token),
		s.newGetCmd(token),
		s.newFilesCmd(token),
		s.newRemindCmd(token),
		s.newCancelCmd(token),
	)

	template := newGroupCmd("template", "List and inspect reusable templates")
	template.AddCommand(
		s.newTemplateListCmd(token),
		s.newTemplateGetCmd(token),
	)

	account := newGroupCmd("account", "Authenticated account identity and quota")
	account.AddCommand(s.newAccountGetCmd(token))

	root.AddCommand(sigReq, template, account)
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
