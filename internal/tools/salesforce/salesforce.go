// Package salesforce is the built-in Salesforce service: a non-interactive
// cobra tree over the Salesforce Platform REST API (/services/data/vXX.0/…)
// rooted at the connected org's instance_url. It covers the CRM teammate
// surface — SOQL query, cross-object search, record CRUD + upsert, sobject
// list/describe, identity, and org limits. Salesforce fails with a non-2xx
// status and a JSON ARRAY error body ([{"errorCode","message"}]); every call
// surfaces both.
package salesforce

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

// DefaultAPIVersion pins the Salesforce REST API version. Winter '26 (v65.0)
// is GA on every production org as of 2026-07; versioned paths keep working
// for years, so a pinned floor is safe. Overridable per-invocation with
// --api-version.
const DefaultAPIVersion = "v65.0"

// Env vars the credential bindings inject (definitions/tools/salesforce.json).
// InstanceURL is the org's My Domain URL and is the mandatory API base — it is
// per-connection state, not a secret, captured at connect time.
const (
	EnvAccessToken = "SALESFORCE_ACCESS_TOKEN"
	EnvInstanceURL = "SALESFORCE_INSTANCE_URL"
)

// Service implements the built-in Salesforce tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides SALESFORCE_INSTANCE_URL; empty = the injected instance
	// URL. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one salesforce subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags/args, unknown subcommands) are exit 2; runtime/API errors
// (Salesforce non-2xx, transport failure) are exit 1. Errors render to stderr
// — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.BaseURL
	if base == "" {
		base = env[EnvInstanceURL]
	}
	if strings.TrimSpace(base) == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvInstanceURL + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base = strings.TrimRight(base, "/")

	root := s.newRoot(token, base)
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

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse credential check).
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

// newRoot builds the resource-grouped cobra tree. query / search / whoami /
// limits are top-level; record and sobject operations hang under a group.
func (s *Service) newRoot(token, base string) *cobra.Command {
	root := &cobra.Command{
		Use:           "salesforce",
		Short:         "Salesforce built-in service (SOQL, records, search, describe)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output (results are always JSON)")
	pf.String("api-version", DefaultAPIVersion, "Salesforce REST API version (e.g. v65.0)")

	c := &client{token: token, base: base, hc: s.hc()}

	record := newGroupCmd("record", "Read and write sObject records")
	record.AddCommand(
		s.newRecordGetCmd(c),
		s.newRecordCreateCmd(c),
		s.newRecordUpdateCmd(c),
		s.newRecordDeleteCmd(c),
		s.newRecordUpsertCmd(c),
	)
	sobject := newGroupCmd("sobject", "Discover objects and fields")
	sobject.AddCommand(
		s.newSObjectListCmd(c),
		s.newSObjectDescribeCmd(c),
	)

	root.AddCommand(
		s.newQueryCmd(c),
		s.newSearchCmd(c),
		s.newWhoamiCmd(c),
		s.newLimitsCmd(c),
		record, sobject,
	)
	return root
}

func (s *Service) hc() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
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

// apiVersion reads the resolved --api-version flag off any command in the tree.
func apiVersion(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("api-version")
	if strings.TrimSpace(v) == "" {
		return DefaultAPIVersion
	}
	return v
}
