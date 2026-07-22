// Package adobesign is the built-in Adobe Acrobat Sign service: a cobra tree
// over the Acrobat Sign REST API v6 (agreements, library documents, transient
// documents). It covers the send / track / retrieve / cancel loop an assistant
// drives: send a document for signature, check agreement + participant status,
// list agreements, download the completed PDF, and cancel a sent agreement.
//
// The account's v6 base host is per-account (the "shard" api_access_point, e.g.
// https://api.na1.adobesign.com/). AnyCLI never performs OAuth and never
// discovers the shard: the host is supplied as the base_uri credential field
// (env ADOBE_SIGN_BASE_URI) by the Helio resolver, and the bearer token as
// access_token (env ADOBE_SIGN_ACCESS_TOKEN). The service composes the v6 base
// path as {base_uri}api/rest/v6 and sends Authorization: Bearer <token>.
package adobesign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvToken and EnvBaseURI are the env vars the credential bindings inject
// (definitions/tools/adobe-sign.json).
const (
	EnvToken   = "ADOBE_SIGN_ACCESS_TOKEN"
	EnvBaseURI = "ADOBE_SIGN_BASE_URI"
)

// apiPath is the v6 REST path appended to the shard base host.
const apiPath = "/api/rest/v6"

// Service implements the built-in Adobe Acrobat Sign tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the shard base host; empty = the ADOBE_SIGN_BASE_URI
	// env value. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *httpClient
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one adobe-sign subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, missing required
// flags, unknown subcommands, unreadable file) are exit 2; runtime/API errors
// (Adobe non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	baseURI := s.BaseURL
	if baseURI == "" {
		baseURI = env[EnvBaseURI]
	}
	if baseURI == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvBaseURI + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, baseURI)
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
// {"error":{"code":"api_error|usage","message":…,"status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"code": "usage", "message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["code"] = "api_error"
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

// newRoot builds the grouped-by-resource cobra tree, mirroring notion/docusign:
// agreement / library / document groups, each a runnable group.
func (s *Service) newRoot(token, baseURI string) *cobra.Command {
	root := &cobra.Command{
		Use:           "adobe-sign",
		Short:         "Adobe Acrobat Sign built-in service (e-signature agreements, REST API v6)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	agreement := newGroupCmd("agreement", "Send, track, retrieve and cancel agreements")
	agreement.AddCommand(
		s.newAgreementSendCmd(token, baseURI),
		s.newAgreementListCmd(token, baseURI),
		s.newAgreementGetCmd(token, baseURI),
		s.newAgreementMembersCmd(token, baseURI),
		s.newAgreementCancelCmd(token, baseURI),
		s.newAgreementDownloadCmd(token, baseURI),
	)
	library := newGroupCmd("library", "List and inspect reusable library documents (templates)")
	library.AddCommand(
		s.newLibraryListCmd(token, baseURI),
		s.newLibraryGetCmd(token, baseURI),
	)
	document := newGroupCmd("document", "Upload raw files to transient storage")
	document.AddCommand(
		s.newDocumentUploadCmd(token, baseURI),
	)

	root.AddCommand(agreement, library, document)
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

// jsonMode reads the global --json flag off any command in the tree.
func jsonMode(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}
