// Package phantombuster is the built-in PhantomBuster service: a
// non-interactive cobra tree over the PhantomBuster v2 REST surface
// (https://api.phantombuster.com/api/v2). Auth is the workspace API key sent in
// the "X-Phantombuster-Key" header (the v2 header; the deprecated v1
// "X-Phantombuster-Key-1" is not used). The key grants full workspace access.
//
// PhantomBuster runs are asynchronous: launching a Phantom (agent) queues a
// container (one execution) and returns a containerId, not a result. The
// command tree mirrors that workflow — discover agents, launch, poll output,
// then fetch the structured result of a container — so an AI teammate can drive
// the whole launch -> poll -> result loop.
//
// Every command emits a provider-neutral JSON envelope on stdout:
// {"ok":true,"data":{...}} on success, {"ok":false,"error":{...}} on failure.
// The raw PhantomBuster object is preserved under data (forward-compatible with
// schema growth); the poll-loop convenience fields (is_running, output_pos) and
// ISO-8601 mirrors of the ms timestamps are added alongside it.
package phantombuster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production PhantomBuster v2 API base.
const DefaultBaseURL = "https://api.phantombuster.com/api/v2"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/phantombuster.json). The PhantomBuster API key is a
// long-lived, non-expiring workspace secret.
const EnvAPIKey = "PHANTOMBUSTER_API_KEY"

// authHeader is the v2 API-key header. The deprecated v1 header
// "X-Phantombuster-Key-1" is intentionally not used.
const authHeader = "X-Phantombuster-Key"

// side_effect annotations (design 318): readOnly for reads (no provider state
// change), writeAction for commands that mutate provider state.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in PhantomBuster tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the PhantomBuster API base; empty = DefaultBaseURL.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one phantombuster subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors
// (PhantomBuster non-2xx, transport failure) are exit 1. Errors render to
// stderr as the {"ok":false,"error":{...}} envelope.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		s.renderError(&apiError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	s.renderError(err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// Every cobra-originated parse/arg/enum/unknown-command error is a usage
	// error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// renderError writes err to stderr as {"ok":false,"error":{"code","status?","message"}}.
// code is "api" for a provider/transport failure (with the HTTP status when
// known) and "usage" for a parse/param error.
func (s *Service) renderError(err error) {
	payload := map[string]any{"code": "usage", "message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["code"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"ok": false, "error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "phantombuster",
		Short:         "PhantomBuster built-in service (async launch -> poll -> result)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	// --json is accepted for uniformity with other tools; output is always the
	// JSON envelope.
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	agent := newGroupCmd("agent", "Manage Phantoms (agents) and their runs")
	agent.AddCommand(
		s.newAgentListCmd(key),
		s.newAgentGetCmd(key),
		s.newAgentLaunchCmd(key),
		s.newAgentOutputCmd(key),
		s.newAgentAbortCmd(key),
	)
	container := newGroupCmd("container", "Inspect Phantom runs (containers)")
	container.AddCommand(
		s.newContainerListCmd(key),
		s.newContainerGetCmd(key),
		s.newContainerOutputCmd(key),
		s.newContainerResultCmd(key),
	)
	org := newGroupCmd("org", "Workspace identity and resources/quota")
	org.AddCommand(
		s.newOrgGetCmd(key),
		s.newOrgResourcesCmd(key),
	)

	root.AddCommand(agent, container, org, s.newMeCmd(key))
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

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
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
