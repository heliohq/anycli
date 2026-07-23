// Package pandadoc is the built-in PandaDoc service: a non-interactive cobra
// tree over the PandaDoc Public API v1 (https://api.pandadoc.com/public/v1).
// It covers the document-workflow / eSignature loop an AI teammate performs —
// send a document from a template, track signing status, retrieve the signed
// PDF, and light template/contact lookup.
//
// Auth is an OAuth bearer access token (Authorization: Bearer <token>) — the
// production Helio path, and the only credential the Helio provider projects.
// PandaDoc fails with a non-2xx status and a JSON body carrying type/detail; a
// 401 rejects the resolved credential so the host can invalidate it.
package pandadoc

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

// DefaultBaseURL is the production PandaDoc Public API base (v1 prefix included).
const DefaultBaseURL = "https://api.pandadoc.com/public/v1"

// readOnly / writeAction are the design-318 anycli.side_effect annotations for
// runnable leaf commands: readOnly for retrieval (no provider state change),
// writeAction for mutations (create/update/delete/send).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/pandadoc.json): the OAuth bearer access token.
const EnvAccessToken = "PANDADOC_ACCESS_TOKEN"

// Service implements the built-in PandaDoc tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the PandaDoc API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one pandadoc subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, missing required
// flags, invalid JSON, unknown subcommands) are exit 2; runtime/API errors
// (PandaDoc non-2xx, transport failure, poll timeout) are exit 1. Errors render
// to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	authz := selectAuth(env)
	if authz == "" {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(authz)
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

// selectAuth builds the Authorization header value from the resolved env: the
// OAuth bearer access token. Returns "" when it is not set.
func selectAuth(env map[string]string) string {
	if tok := env[EnvAccessToken]; tok != "" {
		return "Bearer " + tok
	}
	return ""
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags.
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

// newRoot builds the grouped-by-resource cobra tree. whoami / api are
// top-level; document / template / contact hang under a resource group.
func (s *Service) newRoot(authz string) *cobra.Command {
	root := &cobra.Command{
		Use:           "pandadoc",
		Short:         "PandaDoc built-in service (documents, templates, eSignature)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "print the provider's JSON response instead of concise text")

	document := newGroupCmd("document", "Manage documents")
	document.AddCommand(
		s.newDocumentListCmd(authz),
		s.newDocumentCreateCmd(authz),
		s.newDocumentStatusCmd(authz),
		s.newDocumentDetailsCmd(authz),
		s.newDocumentSendCmd(authz),
		s.newDocumentLinkCmd(authz),
		s.newDocumentDownloadCmd(authz),
		s.newDocumentDeleteCmd(authz),
	)
	template := newGroupCmd("template", "Inspect templates")
	template.AddCommand(
		s.newTemplateListCmd(authz),
		s.newTemplateDetailsCmd(authz),
	)
	contact := newGroupCmd("contact", "Look up or create contacts")
	contact.AddCommand(
		s.newContactListCmd(authz),
		s.newContactCreateCmd(authz),
	)

	root.AddCommand(
		s.newWhoamiCmd(authz),
		s.newAPICmd(authz),
		document, template, contact,
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

// jsonOut reports whether the global --json flag is set on cmd.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}
