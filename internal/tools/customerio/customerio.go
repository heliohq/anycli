// Package customerio is the built-in Customer.io service: a non-interactive
// cobra tree over the Customer.io App API (https://api.customer.io/v1, or the
// EU region https://api-eu.customer.io/v1). Auth is a workspace-scoped App API
// key sent as "Authorization: Bearer <key>". The service wraps the read-and-
// manage surface a messaging-automation teammate uses — people lookup and
// delivery history, campaign/broadcast/newsletter/transactional reporting,
// broadcast triggering, manual-segment lifecycle, transactional send, and
// bulk exports. Customer.io fails with a non-2xx status and a JSON body
// carrying a "meta.error(s)" or "errors" list; every call surfaces the
// provider body. The Track API (a separate site_id:api_key Basic credential)
// is deliberately out of scope.
package customerio

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

// USBaseURL / EUBaseURL are the two official App API regions declared in the
// journeys-app OpenAPI spec's servers block. Paths carry the /v1 prefix.
const (
	USBaseURL = "https://api.customer.io"
	EUBaseURL = "https://api-eu.customer.io"
)

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/customerio.json). The value is a workspace-scoped App
// API key; it is composed into "Authorization: Bearer <key>" by the service.
const EnvAPIKey = "CUSTOMERIO_APP_API_KEY"

// Service implements the built-in Customer.io tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the resolved region base (host only, no /v1); empty
	// means the --region flag decides. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one customer-io subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Customer.io non-2xx, transport failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
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
// missing-key check).
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

// newRoot builds the resource-grouped cobra tree. Every resource is a runnable
// group (bare group shows help; unknown subcommand fails — cobra skips Args
// validation on non-runnable commands, a false success for an agent).
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "customerio",
		Short:         "Customer.io built-in service (App API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output (provider JSON is always emitted)")
	pf.String("region", "us", "App API region: us|eu (maps to api.customer.io / api-eu.customer.io)")

	person := newGroupCmd("person", "Look up people and their messaging history")
	person.AddCommand(
		s.newPersonSearchCmd(key),
		s.newPersonGetCmd(key),
		s.newPersonSegmentsCmd(key),
		s.newPersonMessagesCmd(key),
		s.newPersonActivitiesCmd(key),
	)
	campaign := newGroupCmd("campaign", "Campaign inventory and performance")
	campaign.AddCommand(
		s.newCampaignListCmd(key),
		s.newCampaignGetCmd(key),
		s.newCampaignMetricsCmd(key),
	)
	broadcast := newGroupCmd("broadcast", "API-triggered broadcasts")
	broadcast.AddCommand(
		s.newBroadcastListCmd(key),
		s.newBroadcastGetCmd(key),
		s.newBroadcastMetricsCmd(key),
		s.newBroadcastTriggerCmd(key),
		s.newBroadcastStatusCmd(key),
	)
	segment := newGroupCmd("segment", "Manual-segment inventory and lifecycle")
	segment.AddCommand(
		s.newSegmentListCmd(key),
		s.newSegmentGetCmd(key),
		s.newSegmentCreateCmd(key),
		s.newSegmentDeleteCmd(key),
		s.newSegmentMembersCmd(key),
	)
	newsletter := newGroupCmd("newsletter", "Newsletter inventory and performance")
	newsletter.AddCommand(
		s.newNewsletterListCmd(key),
		s.newNewsletterGetCmd(key),
		s.newNewsletterMetricsCmd(key),
	)
	transactional := newGroupCmd("transactional", "Transactional template inventory and performance")
	transactional.AddCommand(
		s.newTransactionalListCmd(key),
		s.newTransactionalGetCmd(key),
		s.newTransactionalMetricsCmd(key),
	)
	send := newGroupCmd("send", "Send messages")
	send.AddCommand(s.newSendEmailCmd(key))
	message := newGroupCmd("message", "Workspace-wide delivery search")
	message.AddCommand(
		s.newMessageListCmd(key),
		s.newMessageGetCmd(key),
	)
	export := newGroupCmd("export", "Bulk people/delivery exports")
	export.AddCommand(
		s.newExportDeliveriesCmd(key),
		s.newExportPeopleCmd(key),
		s.newExportListCmd(key),
		s.newExportGetCmd(key),
	)
	workspace := newGroupCmd("workspace", "Workspace listing (connectivity check)")
	workspace.AddCommand(s.newWorkspaceListCmd(key))

	root.AddCommand(
		person, campaign, broadcast, segment, newsletter,
		transactional, send, message, export, workspace,
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

// regionBase resolves the API host from the --region flag, honoring a test
// BaseURL override. Only us|eu are accepted — an unknown region is a usage
// error (no silent fallback).
func (s *Service) regionBase(cmd *cobra.Command) (string, error) {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/"), nil
	}
	// The region flag is a persistent flag defined on the root; read it from
	// there so both leaf subcommands (during Execute) and direct callers resolve
	// the same parsed value.
	region, _ := cmd.Root().PersistentFlags().GetString("region")
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "", "us":
		return USBaseURL, nil
	case "eu":
		return EUBaseURL, nil
	default:
		return "", &usageError{msg: fmt.Sprintf("--region %q is invalid: use us or eu", region)}
	}
}
