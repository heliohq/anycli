// Package beehiiv is the built-in beehiiv service: a non-interactive cobra tree
// over the beehiiv v2 REST surface (https://api.beehiiv.com/v2). Auth is
// "Authorization: Bearer <token>", where the token is either an OAuth access
// token or a self-serve API key — both are bearer credentials, so the service
// is auth-mechanism-agnostic. Almost every resource is publication-scoped
// (/publications/{publicationId}/…): a teammate discovers ids via
// `publication list`, then passes --publication-id to resource verbs. beehiiv
// fails with a non-2xx status and a JSON body carrying an errors[].message;
// every command emits the provider JSON on stdout verbatim.
package beehiiv

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

// DefaultBaseURL is the production beehiiv v2 API base.
const DefaultBaseURL = "https://api.beehiiv.com/v2"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/beehiiv.json). It carries a bearer token — an OAuth
// access token or a self-serve API key.
const EnvAPIKey = "BEEHIIV_API_KEY"

// readOnly / writeAction carry the design-318 side-effect annotation for a
// runnable leaf command. readOnly marks side-effect-free reads (GET);
// writeAction marks state changes (subscriber create/update via POST/PUT).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in beehiiv tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the beehiiv API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one beehiiv subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, bad enums, invalid JSON,
// missing required flags, unknown subcommands, malformed publication ids) are
// exit 2; runtime/API errors (beehiiv non-2xx, transport failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIKey]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "BEEHIIV_API_KEY is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. publication is the entry
// point (list/get, unscoped); every other group requires --publication-id.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "beehiiv",
		Short: "beehiiv built-in service (publications, posts, subscribers)",
		Long: "Speaks the beehiiv v2 API and prints its JSON envelope verbatim.\n" +
			"\n" +
			"Almost everything is publication-scoped: one workspace can hold several\n" +
			"publications, and posts, subscribers, segments, tiers and automations\n" +
			"each belong to exactly one. `--publication-id` is therefore required on\n" +
			"every command except `publication list`, and a value not beginning with\n" +
			"`pub_` is rejected before any request goes out — a usage error rather\n" +
			"than a silent 404. Start from `publication list`.\n" +
			"\n" +
			"`--expand` is repeatable and its accepted values differ per resource:\n" +
			"posts take `stats`, `free_web_content` and `recipients`; subscriptions\n" +
			"take `stats`, `custom_fields`, `referrals` and `tags`. Anything not\n" +
			"expanded is absent from the response rather than empty, so a missing\n" +
			"field usually means a missing `--expand`.\n" +
			"\n" +
			"The only writes are `subscription create` and `subscription update`.\n" +
			"Posts cannot be authored or sent through the API at all — that is\n" +
			"app-only — so `post` here is a reporting surface.\n" +
			"\n" +
			"Before writing a subscriber, read the reference lists: `tier list` for\n" +
			"tier names, `custom-field list` for field names, `automation list` for\n" +
			"automation ids. Those names are per publication and beehiiv rejects one\n" +
			"it does not know.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newPublicationCmd(token),
		s.newPostCmd(token),
		s.newSubscriptionCmd(token),
		s.newSegmentCmd(token),
		s.newCustomFieldCmd(token),
		s.newTierCmd(token),
		s.newAutomationCmd(token),
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
