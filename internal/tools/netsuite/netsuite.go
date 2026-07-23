// Package netsuite is the built-in NetSuite service: a cobra tree over the
// SuiteTalk REST Web Services surface (record CRUD + SuiteQL + metadata
// catalog) authenticated with NetSuite Token-Based Authentication (TBA), an
// OAuth 1.0a-style per-request HMAC-SHA256 signature over four secrets plus the
// account id. AnyCLI receives the five TBA values as one opaque JSON payload in
// NETSUITE_CREDENTIALS and owns the decode + signing; Helio stores the payload
// as a single manual credential and never signs.
package netsuite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/netsuite.json): the opaque TBA JSON payload.
const EnvCredentials = "NETSUITE_CREDENTIALS"

// Service implements the built-in NetSuite tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the account-derived SuiteTalk REST base; empty =
	// production host derived from the account id. Tests point it at an
	// httptest server. The OAuth realm always comes from the account id, never
	// this override.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// nowFn / nonceFn override the OAuth timestamp/nonce sources for
	// deterministic signing tests; nil = real time / crypto-random nonce.
	nowFn   func() time.Time
	nonceFn func() string
}

// Execute runs one netsuite subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing/malformed credentials, illegal
// flags, invalid JSON, unknown subcommands) are exit 2; runtime/API errors
// (NetSuite non-2xx incl. 401 credential rejection and 429, transport failure)
// are exit 1. Errors render to stderr — JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	creds, err := decodeCreds(env[EnvCredentials])
	if err != nil {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 2}, nil
	}
	root := s.newRoot(creds)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		jsonMode, _ := root.PersistentFlags().GetBool("json")
		s.renderError(jsonMode, err)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return execution.Failure(err), nil
		}
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
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
// {"error":{"message":…,"kind":"usage|api","status":<HTTP>,"retry_after":<s>}}.
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
		if apiErr.retryAfter != "" {
			payload["retry_after"] = apiErr.retryAfter
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

// emitJSON writes the provider's JSON response to stdout verbatim.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// newRoot builds the cobra tree: top-level query / metadata plus the record
// resource group (get/list/create/update/delete).
func (s *Service) newRoot(creds tbaCreds) *cobra.Command {
	root := &cobra.Command{
		Use:           "netsuite",
		Short:         "NetSuite built-in service (SuiteTalk REST + SuiteQL, Token-Based Auth)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	record := newGroupCmd("record", "Read and write individual records (customer, invoice, salesOrder, …)")
	record.AddCommand(
		s.newRecordGetCmd(creds),
		s.newRecordListCmd(creds),
		s.newRecordCreateCmd(creds),
		s.newRecordUpdateCmd(creds),
		s.newRecordDeleteCmd(creds),
	)
	root.AddCommand(
		s.newQueryCmd(creds),
		s.newMetadataCmd(creds),
		record,
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group shows help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, a
// false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with empty credentials for
// dry-run parsing and traversal (tools.Service seam, design 318). The creds are
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(tbaCreds{}) }
