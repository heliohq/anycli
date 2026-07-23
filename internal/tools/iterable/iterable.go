// Package iterable is the built-in Iterable service: a non-interactive cobra
// tree over the Iterable REST surface (https://api.iterable.com/api and the EU
// twin https://api.eu.iterable.com/api). Auth is a project-scoped key sent in
// the custom "Api-Key" header (never Authorization: Bearer, never in the query
// string or body).
//
// Iterable runs two isolated data centers and a key is bound to exactly one, so
// the region is part of the credential, not a global constant. The single
// injected secret ITERABLE_API_KEY carries "<region>[:<alias>]:<key>" — the
// leading segment selects the data-center host, the optional middle segment is
// a Helio-side account alias that this service ignores, and the last segment is
// the raw key sent as the Api-Key header. A missing/invalid region, a part
// count outside {2,3}, or an empty region/key is a fail-fast usage error (exit
// 2), never a silent default (silent DC-fallback would leak the key to the
// wrong data center).
//
// Iterable reports write results as {"code":"Success","msg":...,"params":...}
// and can return a non-Success code even with a 200 body, so the service checks
// the response code, not HTTP status alone.
package iterable

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

// design-318 side_effect annotation maps shared by every runnable leaf.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/iterable.json). Its value is "<region>[:<alias>]:<key>".
const EnvAPIKey = "ITERABLE_API_KEY"

// Service implements the built-in Iterable tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the resolved data-center base; empty = the region-
	// derived host (us/eu). Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one iterable subcommand with the resolved credential in env.
// Success is exit 0; usage/param errors (bad credential format, missing
// required flags, unknown subcommands, bad enums, invalid JSON) are exit 2;
// runtime/API errors (Iterable non-2xx or non-Success code, transport failure)
// are exit 1. Errors render to stderr — as JSON under --json, plain otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	cred, err := parseCredential(env[EnvAPIKey])
	if err != nil {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 2}, nil
	}

	root := s.newRoot(cred)
	root.SetArgs(args)
	runErr := root.ExecuteContext(ctx)
	if runErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, runErr)

	var apiErr *apiError
	if errors.As(runErr, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(runErr), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse credential
// check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"code":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "code": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["code"] = "api"
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(cred credential) *cobra.Command {
	root := &cobra.Command{
		Use:           "iterable",
		Short:         "Iterable built-in service (cross-channel marketing)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newUserCmd(cred),
		s.newEventCmd(cred),
		s.newListCmd(cred),
		s.newCampaignCmd(cred),
		s.newTemplateCmd(cred),
		s.newEmailCmd(cred),
		s.newCatalogCmd(cred),
	)
	return root
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
