// Package docusign is the built-in DocuSign eSignature service: a
// non-interactive, provider-neutral cobra tree over the eSignature REST API
// v2.1 (https://developers.docusign.com/docs/esign-rest-api/). An AI teammate
// uses it to send envelopes out for signature, track envelope/recipient
// status, download the completed PDF, void envelopes, and list templates.
//
// The API base path is account-specific —
// {base_uri}/restapi/v2.1/accounts/{account_id} — where base_uri is the user's
// region host (e.g. https://na3.docusign.net) and account_id is the API
// account GUID. Both are resolved Helio-side at connect time (from the default
// DocuSign account in /oauth/userinfo) and injected as credential fields, so
// this service is a pure executor and never calls the auth host itself.
//
// DocuSign fails with a non-2xx status and a JSON body carrying
// errorCode/message; 401 rejects the credential. Output is provider-neutral
// snake_case, and --json flips every command to a structured envelope.
package docusign

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

// Credential env vars injected by definitions/tools/docusign.json. All three
// are supplied by the Helio resolver; base_uri + account_id are captured from
// the user's default DocuSign account at connect time.
const (
	EnvAccessToken = "DOCUSIGN_ACCESS_TOKEN"
	EnvBaseURI     = "DOCUSIGN_BASE_URI"
	EnvAccountID   = "DOCUSIGN_ACCOUNT_ID"
)

// apiVersion is the eSignature REST API version segment in the account base
// path. DocuSign pins the current signature API at v2.1.
const apiVersion = "v2.1"

// Service implements the built-in DocuSign tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the account region host (base_uri); empty = the
	// DOCUSIGN_BASE_URI credential value. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one docusign subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (DocuSign non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	accountID := env[EnvAccountID]
	baseURI := s.BaseURL
	if baseURI == "" {
		baseURI = env[EnvBaseURI]
	}
	if missing := firstMissingCredential(token, baseURI, accountID); missing != "" {
		s.renderError(hasJSONArg(args), &usageError{msg: missing + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}

	client := &apiClient{
		baseURI:   baseURI,
		accountID: accountID,
		token:     token,
		hc:        s.HC,
	}
	root := s.newRoot(client)
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

// firstMissingCredential returns the env-var name of the first absent
// credential, or "" when all three are present. Any missing one is fatal —
// there is no degraded path (a call would 401 or hit a malformed base path).
func firstMissingCredential(token, baseURI, accountID string) string {
	switch {
	case token == "":
		return EnvAccessToken
	case baseURI == "":
		return EnvBaseURI
	case accountID == "":
		return EnvAccountID
	}
	return ""
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (e.g. the pre-parse credential check).
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

// newRoot builds the grouped-by-resource cobra tree: envelope (send / list /
// get / recipients / void / download) and template (list / get).
func (s *Service) newRoot(client *apiClient) *cobra.Command {
	root := &cobra.Command{
		Use:           "docusign",
		Short:         "DocuSign eSignature built-in service (send / track / retrieve / void)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	envelope := newGroupCmd("envelope", "Send and track envelopes")
	envelope.AddCommand(
		s.newEnvelopeSendCmd(client),
		s.newEnvelopeListCmd(client),
		s.newEnvelopeGetCmd(client),
		s.newEnvelopeRecipientsCmd(client),
		s.newEnvelopeVoidCmd(client),
		s.newEnvelopeDownloadCmd(client),
	)
	template := newGroupCmd("template", "List reusable templates")
	template.AddCommand(
		s.newTemplateListCmd(client),
		s.newTemplateGetCmd(client),
	)
	root.AddCommand(envelope, template)
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

// jsonMode reads the persistent --json flag from any subcommand.
func jsonMode(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}
