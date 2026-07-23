// Package moz is the built-in Moz service: a non-interactive cobra tree over
// the Moz API (https://api.moz.com/jsonrpc). The Moz API is a single-endpoint
// JSON-RPC 2.0 service — every call is an HTTP POST whose body carries
// {jsonrpc, id, method, params:{data}}, and the desired operation is named by
// the `method` field rather than a URL path. Auth is the account's API token
// in the `x-moz-token` request header (no OAuth; tokens are minted on the Moz
// API dashboard).
//
// Every command builds the method-specific params.data, POSTs it through the
// shared JSON-RPC envelope (client.go), and emits the provider's `result`
// verbatim on stdout (passthrough + newline). A generic `moz call` command
// exposes any method with a raw --data JSON body, so an agent is never blocked
// on a method this tree does not wrap with a typed subcommand.
//
// Exit-code contract (mirrors the notion/semrush/bitly built-ins):
//   - 0 success (result printed verbatim);
//   - 1 runtime/API failure — a JSON-RPC error object, a non-2xx response, or a
//     transport failure — via a typed apiError; HTTP 401/403 (an invalid or
//     revoked token) additionally marks the credential rejected so the host
//     invalidates it;
//   - 2 usage/parse errors (missing required flags, bad --data JSON, unknown
//     subcommands).
//
// Quota safety: every returned row debits the account's monthly row quota and
// Moz meters usage per returned object, so list commands always send an
// explicit page limit (default 25); larger pulls require --limit.
package moz

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

// DefaultBaseURL is the production Moz JSON-RPC endpoint. Every method POSTs to
// this single URL; tests point it at an httptest server.
const DefaultBaseURL = "https://api.moz.com/jsonrpc"

// EnvAPIToken is the env var the credential binding injects
// (definitions/tools/moz.json). Moz API tokens are static account secrets with
// no documented expiry and no refresh cycle (revocation = deleting the token
// in the Moz dashboard).
const EnvAPIToken = "MOZ_API_TOKEN"

// Service implements the built-in Moz tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Moz JSON-RPC endpoint; empty = DefaultBaseURL.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// newRequestID overrides the JSON-RPC request-id generator in tests; nil
	// uses the crypto/rand UUIDv4 default. The Moz API requires an id of at
	// least 24 characters.
	newRequestID func() string
}

// Execute runs one moz subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "MOZ_API_TOKEN is not set"})
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
// {"error":{"message":…,"kind":"usage|api","code":<jsonrpc code>,"status":<HTTP>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.code != 0 {
			payload["code"] = apiErr.code
		}
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

// newRoot builds the grouped-by-resource cobra tree. site/link/keyword/
// ranking-keywords group the resource methods; quota, index, and call are
// top-level.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "moz",
		Short:         "Moz built-in service (JSON-RPC SEO data API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newSiteCmd(token),
		s.newLinkCmd(token),
		s.newKeywordCmd(token),
		s.newRankingKeywordsCmd(token),
		s.newQuotaCmd(token),
		s.newIndexCmd(token),
		s.newCallCmd(token),
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
