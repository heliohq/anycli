// Package freshbooks is the built-in FreshBooks service: a resource-grouped
// cobra tree over the FreshBooks Accounting REST API plus the identity endpoint.
// It injects a single OAuth Bearer token (env FRESHBOOKS_TOKEN), resolves the
// account_id every accounting URL needs from the connected identity, and
// unwraps FreshBooks' nested response envelope into a provider-neutral JSON
// shape so agents never parse the wrapping.
package freshbooks

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

// DefaultBaseURL is the production FreshBooks API base (identity + accounting
// share the same host).
const DefaultBaseURL = "https://api.freshbooks.com"

// apiVersion is the Api-Version header the FreshBooks identity endpoint expects;
// it is harmless on the accounting endpoints, so every call carries it.
const apiVersion = "alpha"

// EnvToken is the env var the credential binding injects (definitions/tools/freshbooks.json).
const EnvToken = "FRESHBOOKS_TOKEN"

// Service implements the built-in FreshBooks tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the FreshBooks API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one freshbooks subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, multi-account
// ambiguity, unknown subcommands) are exit 2; runtime/API errors (FreshBooks
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: "freshbooks: FRESHBOOKS_TOKEN is not set"})
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
		return execution.Failure(err), nil
	}
	// usageError plus every cobra parse/arg/unknown-command error is a usage
	// error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"status":<HTTP or omitted>,"code":<provider or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if apiErr.code != "" {
			payload["code"] = apiErr.code
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

// newRoot builds the resource-grouped cobra tree. me is top-level (identity /
// account discovery); every accounting resource hangs under its own group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "freshbooks",
		Short:         "FreshBooks cloud accounting built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON error output")
	pf.String("account", "", "FreshBooks account id for accounting calls (auto-resolved from the identity when omitted)")

	root.AddCommand(s.newMeCmd(token))
	for _, spec := range resourceSpecs {
		root.AddCommand(s.newResourceGroup(token, spec))
	}
	return root
}

// newMeCmd exposes the raw identity + account mapping so a teammate can discover
// the account_id values every accounting URL needs.
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Show the connected identity and its FreshBooks accounts",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/auth/api/v1/users/me", nil)
			if err != nil {
				return err
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(body, &raw); err != nil {
				return &apiError{msg: fmt.Sprintf("freshbooks: decode identity: %v", err), err: err}
			}
			if resp, ok := raw["response"]; ok {
				return s.emitJSON(resp)
			}
			return s.emitJSON(raw)
		},
	}
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
