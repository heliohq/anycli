// Package ahrefs is the built-in Ahrefs service: a read-only, non-interactive
// cobra tree over the Ahrefs API v3 surface (https://api.ahrefs.com/v3). Auth
// is "Authorization: Bearer <token>" — the same wire header whether the token
// was minted by Ahrefs Connect OAuth or created as a personal API key, so the
// tool is invariant under either credential kind. Ahrefs bills API units per
// (rows × fields), so every rows command ships a curated default `select` and a
// low default `--limit`; the free `usage` command (subscription-info) costs 0
// units and doubles as the connection health probe. Ahrefs fails with a non-2xx
// status and a JSON body of the form {"error": "<message>"}; every command
// surfaces the status and message.
package ahrefs

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

// DefaultBaseURL is the production Ahrefs API v3 base.
const DefaultBaseURL = "https://api.ahrefs.com/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/ahrefs.json). The value is an OAuth access token (Ahrefs
// Connect) or a personal API key — both are sent as a Bearer token.
const EnvAccessToken = "AHREFS_API_TOKEN"

// Service implements the built-in Ahrefs tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Ahrefs API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one ahrefs subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Ahrefs non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "AHREFS_API_TOKEN is not set"})
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

// newRoot builds the resource-grouped cobra tree. Every leaf command emits the
// provider's JSON response verbatim on stdout (domain overview is the one
// tool-merged shape). The tree is read-only end to end.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "ahrefs",
		Short:         "Ahrefs built-in service (SEO data, read-only)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "render errors as a JSON envelope on stderr")

	root.AddCommand(
		s.newDomainCmd(token),
		s.newBacklinksCmd(token),
		s.newRefdomainsCmd(token),
		s.newKeywordsCmd(token),
		s.newPagesCmd(token),
		s.newCompetitorsCmd(token),
		s.newKeywordCmd(token),
		s.newSerpCmd(token),
		s.newBatchCmd(token),
		s.newUsageCmd(token),
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
