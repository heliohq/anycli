// Package mailchimp is the built-in Mailchimp Marketing service: a
// non-interactive cobra tree over the Marketing API v3.0
// (https://<dc>.api.mailchimp.com/3.0). The account's data-center prefix (dc)
// is resolved per invocation — from an API-key suffix, or the OAuth metadata
// endpoint — since it is required to address the API and is not a credential.
// API calls use "Authorization: Bearer <token>" (Mailchimp accepts both API
// keys and OAuth tokens identically). Errors are RFC-7807 problem-detail JSON
// on non-2xx; 401 rejects the credential. Every command emits provider JSON on
// stdout (passthrough + newline); 204 actions emit a small JSON receipt.
package mailchimp

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

// readOnly / writeAction are the design-318 anycli.side_effect annotation maps
// attached to runnable leaf commands: "false" for reads (GET), "true" for
// provider-mutating actions.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/mailchimp.json). The value is a Mailchimp Marketing
// access token (non-expiring OAuth token) or an API key.
const EnvAccessToken = "MAILCHIMP_ACCESS_TOKEN"

// Service implements the built-in Mailchimp tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Marketing API base; empty = resolved per token.
	// Tests point it at an httptest server.
	BaseURL string
	// MetadataURL overrides the OAuth metadata endpoint; empty =
	// defaultMetadataURL. Tests point it at an httptest server.
	MetadataURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mailchimp subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Mailchimp non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
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
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
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

// newRoot builds the grouped-by-resource cobra tree. ping / search live at the
// top level; audience, member, segment, campaign, report, template hang under
// resource groups.
func (s *Service) newRoot(token string) *cobra.Command {
	r := &requester{s: s, token: token}
	root := &cobra.Command{
		Use:           "mailchimp",
		Short:         "Mailchimp Marketing built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newPingCmd(r),
		s.newAudienceCmd(r),
		s.newMemberCmd(r),
		s.newSegmentCmd(r),
		s.newCampaignCmd(r),
		s.newReportCmd(r),
		s.newTemplateCmd(r),
		s.newSearchCmd(r),
	)
	return root
}

// newPingCmd is the health check: GET /ping.
func (s *Service) newPingCmd(r *requester) *cobra.Command {
	return &cobra.Command{
		Use:         "ping",
		Short:       "Health check (GET /ping)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/ping", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
