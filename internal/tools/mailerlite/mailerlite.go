// Package mailerlite is the built-in MailerLite service: a non-interactive
// cobra tree over the MailerLite Connect API (https://connect.mailerlite.com/api).
// Auth is "Authorization: Bearer <token>" (MailerLite account API tokens are
// opaque, non-expiring, all-or-nothing per account). Every request also sends
// Content-Type: application/json and Accept: application/json. The Connect API
// returns a {data, meta, links} envelope for lists and {data} for single
// resources; this tool passes those payloads through verbatim (provider-neutral
// output, design 003 §3). Errors are non-2xx with a JSON body carrying
// message/errors; a 401 {"message":"Unauthenticated."} rejects the credential.
package mailerlite

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

// DefaultBaseURL is the production MailerLite Connect API base (the /api prefix
// is part of the base; resource paths append to it, e.g. /subscribers).
const DefaultBaseURL = "https://connect.mailerlite.com/api"

// EnvAPIToken is the env var the credential binding injects
// (definitions/tools/mailerlite.json). MailerLite account API tokens are opaque
// Bearer strings with no documented expiry.
const EnvAPIToken = "MAILERLITE_API_TOKEN"

// readOnly / writeAction are the design-318 anycli.side_effect annotation maps
// attached to runnable leaf commands: "false" for reads (GET), "true" for
// provider-mutating actions.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in MailerLite tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Connect API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mailerlite subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad enum, invalid JSON, missing
// required flag, unknown subcommand) are exit 2; runtime/API errors (MailerLite
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "MAILERLITE_API_TOKEN is not set"})
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mailerlite",
		Short:         "MailerLite built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newSubscriberCmd(token),
		s.newGroupCmd(token),
		s.newSegmentCmd(token),
		s.newFieldCmd(token),
		s.newCampaignCmd(token),
		s.newFormCmd(token),
		s.newAutomationCmd(token),
		s.newWebhookCmd(token),
	)
	return root
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
