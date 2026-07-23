// Package servicenow is the built-in ServiceNow service: a cobra tree over the
// ServiceNow Table API (https://<instance>.service-now.com/api/now/table/...),
// the single generic REST surface that reads and writes records in any table.
// A thin `incident` convenience group and a raw `api` escape hatch cover
// ergonomics and everything else (Aggregate/Import Set/Attachment APIs) without
// per-endpoint code — the notion precedent (generic passthrough + resource sugar).
//
// Unlike every other built-in service, ServiceNow's target host is per-connection:
// it is derived from the injected instance_url credential, not a constant base.
// Authentication is the instance's Inbound REST API Key sent as the x-sn-apikey
// header (plugin com.glide.tokenbased_auth). See DESIGN.md on tool/servicenow.
package servicenow

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

// EnvInstanceURL is the env var carrying the ServiceNow instance base URL
// (definitions/tools/servicenow.json). It is the per-connection target host.
const EnvInstanceURL = "SERVICENOW_INSTANCE_URL"

// EnvAPIKey is the env var carrying the Inbound REST API Key sent as x-sn-apikey.
const EnvAPIKey = "SERVICENOW_API_KEY"

// apiKeyHeader is the ServiceNow inbound API-key header (com.glide.tokenbased_auth).
const apiKeyHeader = "x-sn-apikey"

// tableAPIPrefix is the Table API path prefix under the instance host.
const tableAPIPrefix = "/api/now/table"

// Service implements the built-in ServiceNow tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the resolved instance base URL (scheme://host, no path);
	// empty = derive from EnvInstanceURL. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one servicenow subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flags, bad enums, invalid
// JSON, missing required flags, unknown subcommands) are exit 2; runtime/API
// errors (ServiceNow non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	instanceURL := s.BaseURL
	if instanceURL == "" {
		instanceURL = env[EnvInstanceURL]
	}
	apiKey := env[EnvAPIKey]

	// Credential checks run before cobra parses flags, so detect --json in the
	// raw args to honor the structured error-envelope contract.
	if instanceURL == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvInstanceURL + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if apiKey == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base, err := normalizeInstanceURL(instanceURL)
	if err != nil {
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(base, apiKey)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
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
	return execution.Result{}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse credential
// checks).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape mirrors ServiceNow's
// own envelope: {"error":{"message":…,"detail":…,"status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	inner := map[string]any{"message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.status != 0 {
			inner["status"] = apiErr.status
		}
		if apiErr.detail != "" {
			inner["detail"] = apiErr.detail
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": inner})
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

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (GET / query), "true" for
// provider-state mutations (create / update / delete / resolve / raw api).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// newRoot builds the cobra tree: table (generic CRUD), incident (sugar), whoami
// (verify + identity), and api (raw escape hatch).
func (s *Service) newRoot(base, apiKey string) *cobra.Command {
	root := &cobra.Command{
		Use:           "servicenow",
		Short:         "ServiceNow built-in service (Table API: incidents, tasks, CMDB, knowledge)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output (also the error-envelope format)")

	c := &client{base: base, apiKey: apiKey, hc: s.HC}

	table := newGroupCmd("table", "Generic Table API CRUD on any table")
	table.AddCommand(
		s.newTableQueryCmd(c),
		s.newTableGetCmd(c),
		s.newTableCreateCmd(c),
		s.newTableUpdateCmd(c),
		s.newTableDeleteCmd(c),
	)
	incident := newGroupCmd("incident", "Incident convenience commands (accepts INC number or sys_id)")
	incident.AddCommand(
		s.newIncidentListCmd(c),
		s.newIncidentGetCmd(c),
		s.newIncidentCreateCmd(c),
		s.newIncidentUpdateCmd(c),
		s.newIncidentResolveCmd(c),
	)

	root.AddCommand(table, incident, s.newWhoamiCmd(c), s.newAPICmd(c))
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
