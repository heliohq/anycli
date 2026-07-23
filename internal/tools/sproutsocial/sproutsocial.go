// Package sproutsocial is the built-in Sprout Social service: a non-interactive
// cobra tree over the Sprout Social public API (https://api.sproutsocial.com).
//
// Auth is a single account-scoped API Access Token (a long-lived, non-expiring
// bearer secret) sent as "Authorization: Bearer <token>". Every path is
// /v1/<customer_id>/<resource> except the discovery endpoint
// /v1/metadata/client, which carries no customer id and returns the customer
// ids the token can see. The customer id is injected from the environment
// (SPROUT_SOCIAL_CUSTOMER_ID) and can be overridden per-invocation with the
// global --customer-id flag.
//
// Sprout wraps every response in a JSON envelope { "data": …, "paging"?: …,
// "error"?: … }. Analytics / messages / cases are POST-with-a-filter-body
// endpoints using Sprout's filter DSL (e.g. created_time.in(2026-01-01...2026-02-01));
// the DSL is not modeled as flags — each POST verb takes thin ergonomic flags
// plus a --body raw-passthrough escape hatch so any documented Sprout query can
// be sent verbatim. Output is the Sprout envelope passed through unmodified so
// the caller can page (paging.next_cursor / paging.total_pages) and read every
// field; --json is accepted for uniformity.
package sproutsocial

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

// DefaultBaseURL is the production Sprout Social API base (no version segment;
// paths carry /v1/…).
const DefaultBaseURL = "https://api.sproutsocial.com"

// EnvToken is the env var the credential binding injects
// (definitions/tools/sprout-social.json). It is the account-scoped API Access
// Token, a long-lived non-expiring bearer secret.
const EnvToken = "SPROUT_SOCIAL_TOKEN"

// EnvCustomerID is the env var carrying the connection's default customer id,
// used to fill the /v1/{cid}/… path segment. The global --customer-id flag
// overrides it for tokens that can see multiple customers.
const EnvCustomerID = "SPROUT_SOCIAL_CUSTOMER_ID"

// Service implements the built-in Sprout Social tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Sprout API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one sprout-social subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (missing required flags, invalid
// JSON, bad subcommands, an unresolved customer id) are exit 2; runtime/API
// errors (Sprout non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "SPROUT_SOCIAL_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token, env[EnvCustomerID])
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
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>,"request_id":<Sprout X-Sprout-Request-ID or omitted>}}.
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
		if apiErr.requestID != "" {
			payload["request_id"] = apiErr.requestID
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

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (analytics/inbox/cases queries and
// metadata GETs), "true" for the draft-post creation that mutates provider state.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// newRoot builds the resource-grouped cobra tree. The global --customer-id flag
// defaults to the env-injected customer id and overrides it per invocation.
func (s *Service) newRoot(token, defaultCustomerID string) *cobra.Command {
	root := &cobra.Command{
		Use:           "sprout-social",
		Short:         "Sprout Social built-in service (analytics, inbox, publishing)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "output JSON (always on; accepted for uniformity)")
	pf.String("customer-id", defaultCustomerID, "override the injected Sprout customer id")

	root.AddCommand(
		s.newMetadataCmd(token),
		s.newAnalyticsCmd(token),
		s.newMessagesCmd(token),
		s.newCasesCmd(token),
		s.newPublishingCmd(token),
	)
	return root
}

// resolveCID reads the effective customer id (--customer-id flag, defaulted from
// the env at newRoot time). An empty value is a usage error — every path except
// `metadata client` needs it.
func resolveCID(cmd *cobra.Command) (string, error) {
	cid, _ := cmd.Flags().GetString("customer-id")
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return "", &usageError{msg: "no Sprout customer id: set SPROUT_SOCIAL_CUSTOMER_ID or pass --customer-id (use `metadata client` to list ids the token can see)"}
	}
	return cid, nil
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
