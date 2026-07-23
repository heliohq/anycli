// Package kustomer is the built-in Kustomer service: a cobra tree over the
// Kustomer REST v1 surface (support CRM — customers, conversations, messages,
// notes, and search). Kustomer returns JSON:API-shaped envelopes
// ({"data":…,"meta":…,"links":…}); every read passes the provider JSON through
// verbatim on stdout so an agent can page via links.next / meta.pagination.
//
// Base URL & pod routing. Kustomer runs multiple production pods and a token is
// minted on one pod. The org-subdomain host
// https://{orgname}.api.kustomerapp.com/v1 routes a request to the pod that
// minted the token; the generic host https://api.kustomerapp.com/v1 only works
// for orgs on the default pod and otherwise fails with a pod-routing error.
//
// The default base is the generic host. When KUSTOMER_ORG_NAME is set the tool
// builds the pod-routed org-subdomain host instead. KUSTOMER_ORG_NAME is a
// staged seam: the Kustomer OAuth token response carries no org identifier
// (verified), so Helio cannot yet capture the orgname to supply it — a
// credential wiring it is deferred until that capture capability lands. Until
// then the tool runs on the generic host (correct for default-pod orgs), and
// L2/operator runs may set KUSTOMER_ORG_NAME directly to exercise pod routing.
package kustomer

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

// DefaultBaseURL is the generic (default-pod-only) Kustomer API base used when
// KUSTOMER_ORG_NAME is not supplied. Pod-routed orgs need the org-subdomain
// host built by resolveBaseURL.
const DefaultBaseURL = "https://api.kustomerapp.com/v1"

// apiHostSuffix is appended to the org subdomain to build the pod-routed base.
const apiHostSuffix = ".api.kustomerapp.com/v1"

// EnvToken is the env var the credential binding injects with the bearer token
// (definitions/tools/kustomer.json access_token → KUSTOMER_API_TOKEN).
const EnvToken = "KUSTOMER_API_TOKEN"

// EnvOrgName is the env var carrying the org subdomain for pod routing.
// Optional: absent selects DefaultBaseURL (the generic host). No Helio
// credential wires it yet (see the package doc); it is an operator/L2 seam.
const EnvOrgName = "KUSTOMER_ORG_NAME"

// resolveBaseURL builds the API base from the org subdomain. An empty orgName
// yields the generic (default-pod) host; a set orgName yields the pod-routed
// org-subdomain host.
func resolveBaseURL(orgName string) string {
	orgName = strings.TrimSpace(orgName)
	if orgName == "" {
		return DefaultBaseURL
	}
	return "https://" + orgName + apiHostSuffix
}

// Service implements the built-in Kustomer tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the resolved API base; empty = resolve from
	// KUSTOMER_ORG_NAME (or DefaultBaseURL). Tests point it at an httptest
	// server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one kustomer subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flags, invalid JSON, missing
// required args, unknown subcommands) are exit 2; runtime/API errors (Kustomer
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &configError{msg: "KUSTOMER_API_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.BaseURL
	if base == "" {
		base = resolveBaseURL(env[EnvOrgName])
	}
	root := s.newRoot(base, token)
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
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

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api|config","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	var cfgErr *configError
	switch {
	case errors.As(err, &apiErr):
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	case errors.As(err, &cfgErr):
		payload["kind"] = "config"
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(base, token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "kustomer",
		Short:         "Kustomer built-in service (support CRM)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")

	customer := newGroupCmd("customer", "Look up and create customers")
	customer.AddCommand(
		s.newCustomerGetCmd(base, token),
		s.newCustomerGetByEmailCmd(base, token),
		s.newCustomerConversationsCmd(base, token),
		s.newCustomerCreateCmd(base, token),
	)
	conversation := newGroupCmd("conversation", "Read and manage conversations (tickets)")
	conversation.AddCommand(
		s.newConversationGetCmd(base, token),
		s.newConversationListCmd(base, token),
		s.newConversationCreateCmd(base, token),
		s.newConversationUpdateCmd(base, token),
	)
	message := newGroupCmd("message", "Read and post conversation messages")
	message.AddCommand(
		s.newMessageListCmd(base, token),
		s.newMessageCreateCmd(base, token),
	)
	note := newGroupCmd("note", "Read and add internal notes")
	note.AddCommand(
		s.newNoteListCmd(base, token),
		s.newNoteCreateCmd(base, token),
	)
	search := newGroupCmd("search", "Free-form resource search")
	search.AddCommand(
		s.newSearchCustomersCmd(base, token),
	)

	root.AddCommand(customer, conversation, message, note, search)
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
