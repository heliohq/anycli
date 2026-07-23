// Package paddle is the built-in Paddle Billing service: a grouped-by-resource
// cobra tree over the Paddle Billing management API (api.paddle.com). It wraps
// Paddle Billing only — the legacy Paddle Classic vendor API is out of scope.
// Auth is a single Bearer API key injected as PADDLE_API_KEY; the key's prefix
// (pdl_live_… / pdl_sdbx_…) selects the live or sandbox base URL, so the caller
// never supplies one. Every call pins the Paddle-Version header and passes the
// provider's {data, meta} envelope through unchanged.
package paddle

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

// EnvToken is the env var the credential binding injects
// (definitions/tools/paddle.json).
const EnvToken = "PADDLE_API_KEY"

// EnvEnvironment optionally overrides the environment for legacy unstructured
// keys that do not encode it in their prefix (default live; "sandbox" routes to
// the sandbox base URL).
const EnvEnvironment = "PADDLE_ENV"

// Service implements the built-in Paddle tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// baseURL overrides the environment-routed base; empty = prefix routing.
	// Tests point it at an httptest server.
	baseURL string
	// env is the PADDLE_ENV override captured at Execute time (legacy-key
	// routing). Empty outside a run.
	env string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// SetBaseURL overrides the environment-routed base URL. Test-only seam.
func (s *Service) SetBaseURL(base string) { s.baseURL = base }

// Execute runs one paddle subcommand with the resolved credentials in env.
// Exit 0 = success; usage/param errors (bad flags, invalid JSON, unknown
// subcommands) = exit 2; runtime/API errors (Paddle non-2xx, transport) = exit
// 1. Errors render to stderr — JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), newUsageError("%s is not set", EnvToken))
		return execution.Result{ExitCode: 1}, nil
	}
	s.env = env[EnvEnvironment]
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
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags.
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
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

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message","kind","status?","code?","detail?","documentation_url?","request_id?","retry_after?","errors?"}}.
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
		if apiErr.detail.Code != "" {
			payload["code"] = apiErr.detail.Code
		}
		if apiErr.detail.Detail != "" {
			payload["detail"] = apiErr.detail.Detail
		}
		if apiErr.detail.DocumentationURL != "" {
			payload["documentation_url"] = apiErr.detail.DocumentationURL
		}
		if apiErr.requestID != "" {
			payload["request_id"] = apiErr.requestID
		}
		if apiErr.retryAfter != "" {
			payload["retry_after"] = apiErr.retryAfter
		}
		if len(apiErr.detail.Errors) > 0 {
			payload["errors"] = apiErr.detail.Errors
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

// emit writes a successful response. Default mode prints the response `data`
// (indented) so agents get the resource verbatim; --json prints the full
// {data, meta} envelope compactly so agents can page via meta.pagination.next.
func (s *Service) emit(jsonMode bool, env *successEnvelope) error {
	if jsonMode {
		out := map[string]json.RawMessage{"data": env.Data}
		if len(env.Meta) > 0 {
			out["meta"] = env.Meta
		}
		b, err := json.Marshal(out)
		if err != nil {
			return &apiError{err: fmt.Errorf("encode output: %w", err)}
		}
		_, err = fmt.Fprintln(s.stdout(), string(b))
		return err
	}
	var pretty any
	if err := json.Unmarshal(env.Data, &pretty); err != nil {
		// Non-JSON data (e.g. an invoice URL string still arrives as JSON);
		// fall back to the raw bytes.
		_, err := fmt.Fprintln(s.stdout(), string(env.Data))
		return err
	}
	b, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		return &apiError{err: fmt.Errorf("encode output: %w", err)}
	}
	_, err = fmt.Fprintln(s.stdout(), string(b))
	return err
}

// newGroupCmd is a runnable command group: a bare group shows help (exit 0), an
// unknown subcommand fails — matching the design-318 group contract.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full tree built with an empty token for dry-run
// parsing and traversal (tools.Service seam, design 318).
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
