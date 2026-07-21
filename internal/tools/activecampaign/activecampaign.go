// Package activecampaign is the built-in ActiveCampaign service: a verb-first
// cobra tree over the ActiveCampaign v3 REST API (contacts, lists, tags,
// deals, pipelines, campaigns, automations, custom fields). Authentication is
// the per-request Api-Token header; the account base URL is per-account
// (https://<account>.api-us1.com/api/3/) and is supplied out of band, so this
// service takes two injected credentials — the token and the account URL.
//
// Output is the provider's JSON body verbatim on stdout. A non-2xx response
// surfaces the provider's message as an apiError carrying the HTTP status;
// 401/403 additionally classify as a credential rejection so the host can
// invalidate a bad key.
package activecampaign

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

// EnvToken is the env var the credential binding injects for the Api-Token
// secret (definitions/tools/activecampaign.json).
const EnvToken = "ACTIVECAMPAIGN_API_TOKEN"

// EnvURL is the env var carrying the per-account base URL (non-secret account
// input, projected from the connection account key).
const EnvURL = "ACTIVECAMPAIGN_API_URL"

// Service implements the built-in ActiveCampaign tool. It satisfies
// tools.Service by duck typing (this package never imports the registry).
type Service struct {
	// BaseURL overrides the account base; empty = normalize EnvURL from env.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one activecampaign subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (bad flags, invalid JSON, unknown
// subcommand) are exit 2; runtime/API errors (non-2xx, transport failure) are
// exit 1, with 401/403 additionally flagged as a credential rejection. Errors
// render to stderr — JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base, err := normalizeBaseURL(s.baseURLInput(env))
	if err != nil {
		s.renderError(hasJSONArg(args), &usageError{msg: fmt.Sprintf("%s is invalid: %v", EnvURL, err)})
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, base)
	root.SetArgs(args)
	execErr := root.ExecuteContext(ctx)
	if execErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, execErr)

	var apiErr *apiError
	if errors.As(execErr, &apiErr) {
		return execution.Failure(execErr), nil
	}
	return execution.Result{ExitCode: 2}, nil
}

// baseURLInput picks the account URL: an explicit BaseURL override (tests) wins
// over the injected EnvURL.
func (s *Service) baseURLInput(env map[string]string) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return env[EnvURL]
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (pre-parse credential checks).
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

// newRoot builds the resource-grouped cobra tree.
func (s *Service) newRoot(token, base string) *cobra.Command {
	root := &cobra.Command{
		Use:           "activecampaign",
		Short:         "ActiveCampaign built-in service (marketing automation & CRM, API v3)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	c := &client{base: base, token: token, hc: s.HC, out: s.stdout()}

	contact := newGroupCmd("contact", "Manage contacts")
	contact.AddCommand(
		s.newContactListCmd(c),
		s.newContactGetCmd(c),
		s.newContactCreateCmd(c),
		s.newContactUpdateCmd(c),
		s.newContactDeleteCmd(c),
		s.newContactSubscribeCmd(c),
		s.newContactTagCmd(c),
		s.newContactUntagCmd(c),
		s.newContactAutomateCmd(c),
	)

	list := newGroupCmd("list", "Discover audiences (lists)")
	list.AddCommand(
		s.newSimpleListCmd(c, "lists"),
		s.newSimpleGetCmd(c, "lists"),
	)

	tag := newGroupCmd("tag", "Discover and create tags")
	tag.AddCommand(
		s.newSimpleListCmd(c, "tags"),
		s.newTagCreateCmd(c),
	)

	deal := newGroupCmd("deal", "CRM deals")
	deal.AddCommand(
		s.newSimpleListCmd(c, "deals"),
		s.newSimpleGetCmd(c, "deals"),
		s.newResourceCreateCmd(c, "deal", "deals"),
		s.newResourceUpdateCmd(c, "deal", "deals"),
	)

	pipeline := newGroupCmd("pipeline", "CRM pipelines (deal groups)")
	pipeline.AddCommand(s.newSimpleListCmd(c, "dealGroups"))

	stage := newGroupCmd("stage", "CRM pipeline stages (deal stages)")
	stage.AddCommand(s.newSimpleListCmd(c, "dealStages"))

	campaign := newGroupCmd("campaign", "Campaign reporting")
	campaign.AddCommand(
		s.newSimpleListCmd(c, "campaigns"),
		s.newSimpleGetCmd(c, "campaigns"),
	)

	automation := newGroupCmd("automation", "Discover automations")
	automation.AddCommand(s.newSimpleListCmd(c, "automations"))

	field := newGroupCmd("field", "Custom contact fields")
	field.AddCommand(s.newSimpleListCmd(c, "fields"))

	account := newGroupCmd("account", "B2B accounts")
	account.AddCommand(s.newSimpleListCmd(c, "accounts"))

	root.AddCommand(contact, list, tag, deal, pipeline, stage, campaign, automation, field, account)
	return root
}

// newGroupCmd is a runnable command group: a bare group prints help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, which
// would let an unknown subcommand exit 0 — a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
