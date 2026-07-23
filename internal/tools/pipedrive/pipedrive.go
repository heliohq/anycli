// Package pipedrive is the built-in Pipedrive CRM service: a cobra tree over
// the Pipedrive REST API, driven by an OAuth 2.0 access token. Unlike a
// fixed-host provider, Pipedrive scopes every account to a per-company base URL
// (api_domain, e.g. https://acme.pipedrive.com) returned in the token response;
// that base URL is delivered to the service as the PIPEDRIVE_API_DOMAIN
// credential and every request is built from it — there is no fallback host.
//
// The tool is v2-first: Deals, Persons, Organizations, Activities,
// Pipelines/Stages, and search ride /api/v2 (cursor pagination, PATCH updates).
// Leads, Notes, and Users have no v2 equivalent and stay on /api/v1 (offset
// pagination). Successful responses are written to stdout verbatim; failures
// surface Pipedrive's own error/error_info on stderr.
package pipedrive

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

const (
	// EnvAccessToken and EnvAPIDomain are populated by the credential bindings
	// in definitions/tools/pipedrive.json. EnvAPIDomain carries the per-company
	// base URL (the OAuth token response's api_domain), used verbatim as the
	// API base — the tool never assumes a fixed pipedrive.com host.
	EnvAccessToken = "PIPEDRIVE_ACCESS_TOKEN"
	EnvAPIDomain   = "PIPEDRIVE_API_DOMAIN"
)

// anycli.side_effect annotations (design 318): readOnly marks a leaf command
// that only reads provider state; writeAction marks one that mutates it.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Pipedrive tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one pipedrive subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required args,
// unknown subcommands) are exit 2; runtime/API errors (Pipedrive non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	jsonMode := hasJSONArg(args)

	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(jsonMode, &usageError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base, err := normalizeAPIDomain(env[EnvAPIDomain])
	if err != nil {
		s.renderError(jsonMode, err)
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(base, token)
	root.SetArgs(args)
	if runErr := root.ExecuteContext(ctx); runErr != nil {
		mode, _ := root.PersistentFlags().GetBool("json")
		s.renderError(mode || jsonMode, runErr)
		var apiErr *apiError
		if errors.As(runErr, &apiErr) {
			return execution.Failure(runErr), nil
		}
		// usageError plus every cobra parse/arg/unknown-command error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
}

// normalizeAPIDomain validates the api_domain credential and returns the base
// URL with any trailing slash trimmed. It fails explicitly (no fallback host)
// when the value is missing or is not an absolute https URL.
func normalizeAPIDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: EnvAPIDomain + " is not set"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", &usageError{msg: fmt.Sprintf(
			"%s is not an absolute URL: %q (expected e.g. https://<company>.pipedrive.com)", EnvAPIDomain, raw)}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &usageError{msg: fmt.Sprintf(
			"%s must be an http(s) URL: %q", EnvAPIDomain, raw)}
	}
	return strings.TrimRight(raw, "/"), nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// credential checks).
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

// newRoot builds the grouped-by-resource cobra tree. search is top-level
// (cross-entity); everything else hangs under its resource group.
func (s *Service) newRoot(base, token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "pipedrive",
		Short:         "Pipedrive CRM built-in service (OAuth, v2-first)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	c := &caller{s: s, base: base, token: token}
	root.AddCommand(
		s.newDealGroup(c),
		s.newPersonGroup(c),
		s.newOrgGroup(c),
		s.newActivityGroup(c),
		s.newLeadGroup(c),
		s.newNoteGroup(c),
		s.newPipelineGroup(c),
		s.newStageGroup(c),
		s.newUserGroup(c),
		s.newSearchCmd(c),
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
