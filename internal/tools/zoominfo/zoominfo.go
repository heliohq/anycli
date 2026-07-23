// Package zoominfo is the built-in ZoomInfo service: a cobra tree over the
// ZoomInfo Enterprise API (https://api.zoominfo.com) covering the B2B
// sales-intelligence workflow an AI teammate needs — find (search) and enrich
// people and companies, discover valid request fields (lookup), and check
// remaining credits (usage).
//
// ZoomInfo's recommended flow is two-stage Search -> Enrich: Search finds
// candidate record IDs and consumes NO credit; Enrich pulls full profiles and
// CONSUMES A CREDIT per newly enriched record. Commands take the request body
// as JSON passthrough (--body / --file) so the surface stays valid as ZoomInfo
// migrates its Legacy Enterprise API to the New API without a field-by-field
// rebuild; the lookup command lets the AI discover valid filters/outputFields.
//
// Authentication is ZoomInfo's proprietary PKI JWT exchange (see auth.go): the
// service mints an RS256 client-assertion in-process, exchanges it at
// /authenticate for a ~60-minute access JWT (no refresh token), then calls the
// requested data endpoint. One exchange per invocation.
package zoominfo

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

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/zoominfo.json). It carries a JSON object
// {username, client_id, private_key} — Helio packs the three connect-form
// fields into one secret because the vault face stores a single secret.
const EnvCredentials = "ZOOMINFO_CREDENTIALS"

// readOnly carries the design-318 anycli.side_effect="false" annotation. Every
// zoominfo leaf is a read: search and enrich are B2B data lookups (enrich
// consumes credits but does not mutate provider state), and lookup/usage are
// pure reads — so no writeAction counterpart is needed.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// Service implements the built-in ZoomInfo tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the ZoomInfo API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// runState memoizes the per-invocation access JWT so a multi-step command tree
// authenticates exactly once (ZoomInfo caps /authenticate at 1 req/sec).
type runState struct {
	creds credentials
	token string
}

// Execute runs one zoominfo subcommand with the resolved credential in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON body,
// misconfigured credential, unknown subcommand) are exit 2; runtime/API errors
// (ZoomInfo non-2xx, transport failure) are exit 1. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	creds, err := parseCredentials(env[EnvCredentials])
	if err != nil {
		// Pre-parse credential check: detect --json in raw args to honor the
		// structured error-envelope contract before cobra parses flags.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 2}, nil
	}
	st := &runState{creds: creds}
	root := s.newRoot(st)
	root.SetArgs(args)
	runErr := root.ExecuteContext(ctx)
	if runErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, runErr)

	var apiErr *apiError
	if errors.As(runErr, &apiErr) {
		return execution.Failure(runErr), nil
	}
	// usageError plus every cobra parse/arg/unknown-command error is a usage
	// error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// accessToken lazily authenticates once per invocation and memoizes the JWT.
func (s *Service) accessToken(ctx context.Context, st *runState) (string, error) {
	if st.token != "" {
		return st.token, nil
	}
	token, err := s.authenticate(ctx, st.creds)
	if err != nil {
		return "", err
	}
	st.token = token
	return token, nil
}

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

// emit writes a successful response body to stdout verbatim (it is already the
// provider's JSON).
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// newRoot builds the grouped-by-resource cobra tree: contact/company groups
// each carry search+enrich; lookup and usage are top-level.
func (s *Service) newRoot(st *runState) *cobra.Command {
	root := &cobra.Command{
		Use:           "zoominfo",
		Short:         "ZoomInfo B2B sales intelligence (search + enrich people and companies)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	contact := newGroupCmd("contact", "Search and enrich people")
	contact.AddCommand(
		s.newBodyCmd(st, "search", "Find contact candidates by title/company/location (no credit)", "/search/contact"),
		s.newBodyCmd(st, "enrich", "Enrich up to 25 contacts by id or match keys (CONSUMES CREDITS)", "/enrich/contact"),
	)
	company := newGroupCmd("company", "Search and enrich companies")
	company.AddCommand(
		s.newBodyCmd(st, "search", "Find company candidates by name/domain/industry (no credit)", "/search/company"),
		s.newBodyCmd(st, "enrich", "Enrich up to 25 companies by id (CONSUMES CREDITS)", "/enrich/company"),
	)
	root.AddCommand(contact, company, s.newLookupCmd(st), s.newUsageCmd(st))
	return root
}

// newGroupCmd is a runnable command group: a bare group shows help (exit 0),
// an unknown subcommand fails — cobra skips Args validation on non-runnable
// commands, which would let an unknown subcommand exit 0 (a false success).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
