// Package omnisend is the built-in Omnisend service: a non-interactive cobra
// tree over Omnisend's dated API line (https://api.omnisend.com/api). Auth is
// an OAuth bearer token ("Authorization: Bearer <token>") plus the pinned
// "Omnisend-Version" header emitted on every call. Omnisend fails with a
// non-2xx status and a JSON body carrying an error message; a 401 rejects the
// resolved credential. Reads (list/get) are structured commands; writes
// (create/update/event) take a raw --data JSON body passed through verbatim,
// so the tool never guesses Omnisend's nested request schemas. Cursor
// pagination is surfaced by passing the JSON through unchanged — the caller
// reads paging.cursors.after and replays it via --after.
package omnisend

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

// DefaultBaseURL is the production Omnisend dated-line API base.
const DefaultBaseURL = "https://api.omnisend.com/api"

// omnisendVersion is the API version pinned by every built-in Omnisend call.
// It is a constant emitted by this service, not a per-account credential.
const omnisendVersion = "2026-03-15"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/omnisend.json). Omnisend OAuth access tokens are
// effectively non-expiring.
const EnvAccessToken = "OMNISEND_ACCESS_TOKEN"

// readOnly / writeAction are the design-318 anycli.side_effect annotations for
// runnable leaf commands: false = no provider state change (GET/list/get),
// true = mutates provider state (create/update/send).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Omnisend tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Omnisend API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one omnisend subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad enums,
// invalid JSON, unknown subcommands) are exit 2; runtime/API errors (Omnisend
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "OMNISEND_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		jsonMode, _ := root.PersistentFlags().GetBool("json")
		s.renderError(jsonMode, err)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return execution.Failure(err), nil
		}
		// usageError plus every cobra-originated parse/arg/enum/unknown-command
		// error is inherently a usage error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
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
		Use:           "omnisend",
		Short:         "Omnisend built-in service (dated API line)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newContactCmd(token),
		s.newEventCmd(token),
		s.newCampaignCmd(token),
		s.newSegmentCmd(token),
		s.newProductCmd(token),
		s.newBatchCmd(token),
		s.newBrandCmd(token),
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
