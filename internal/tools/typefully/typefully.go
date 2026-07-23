// Package typefully is the built-in Typefully service: a non-interactive cobra
// tree over the Typefully v2 REST surface (https://api.typefully.com/v2). Auth
// is "Authorization: Bearer <key>" (v2 renamed v1's x-api-key header while
// keeping the Bearer scheme). Every command emits the provider JSON verbatim on
// stdout (passthrough + newline).
//
// Exit-code contract: 0 success; 2 for usage/param errors (illegal flag combos,
// missing required flags, invalid JSON, unknown subcommands); 1 for runtime/API
// errors (Typefully non-2xx, transport failure). A 401 (and an unambiguously
// auth-related 403) marks the credential rejected; a permission-scoped 403 (a
// valid key lacking the required per-social-set access level) and a 429
// rate-limit are ordinary runtime errors that do NOT reject the credential.
package typefully

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

// DefaultBaseURL is the production Typefully v2 API base.
const DefaultBaseURL = "https://api.typefully.com/v2"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/typefully.json). Typefully v2 keys are non-expiring,
// user-scoped bearer tokens created in Settings -> API.
const EnvAPIKey = "TYPEFULLY_API_KEY"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaves: "false" for side-effect-free reads (GET), "true" for
// provider-state mutations (POST/PATCH/PUT/DELETE).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Typefully tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Typefully API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one typefully subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIKey]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "TYPEFULLY_API_KEY is not set"})
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
	// error is inherently a usage error -> exit 2.
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
// {"error":{"message":…,"kind":"usage|api|permission|rate_limit","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = apiErr.kind
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "typefully",
		Short:         "Typefully built-in service (drafts, scheduling, queue, analytics)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (raw provider JSON is always emitted)")

	root.AddCommand(
		s.newMeCmd(token),
		s.newSocialSetCmd(token),
		s.newDraftCmd(token),
		s.newTagCmd(token),
		s.newQueueCmd(token),
		s.newAnalyticsCmd(token),
		s.newMediaCmd(token),
		s.newLinkedInCmd(token),
		s.newCommentCmd(token),
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
