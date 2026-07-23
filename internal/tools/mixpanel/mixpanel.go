// Package mixpanel is the built-in Mixpanel service: a read-only, cobra-tree
// wrapper over the Mixpanel Query, Lexicon Schemas, Raw Data Export, and App
// APIs (design row 117). An AI teammate uses it to read product analytics —
// segmentation, funnels, retention, events, cohorts, people — never to ingest
// events (that is the app's own instrumentation job, on a different credential).
//
// Authentication is HTTP Basic auth with a Service Account username:secret
// pair; project_id is injected from the credential (not a per-call flag); and
// the API host is built from the region (us/eu/in) because Mixpanel enforces
// data residency on every surface. All four values arrive as env vars from the
// definition's credential bindings.
package mixpanel

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

// readOnly marks a leaf command as having no provider side effect (design 318).
var readOnly = map[string]string{"anycli.side_effect": "false"}

// EnvCredentials is the single env var the credential binding injects
// (definitions/tools/mixpanel.json). It carries a JSON object packing the four
// Service Account values, because Helio's manual_credentials storage is a
// single-secret token payload (design 317 D5/D8: one token.access_token, no
// per-field CredentialSource). Shape:
//
//	{"username":"…","secret":"…","project_id":"3193719","region":"us"}
//
// region is optional (defaults to "us"). The service parses this into the four
// discrete values it needs and never logs the secret.
const EnvCredentials = "MIXPANEL_CREDENTIALS"

// credentials is the parsed MIXPANEL_CREDENTIALS payload.
type credentials struct {
	Username  string `json:"username"`
	Secret    string `json:"secret"`
	ProjectID string `json:"project_id"`
	Region    string `json:"region"`
}

// Service implements the built-in Mixpanel tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// QueryBaseURL / AppBaseURL / ExportBaseURL override the region-derived
	// host bases; empty = derive from MIXPANEL_REGION. Tests point them at an
	// httptest server. Each maps to one Mixpanel host family.
	QueryBaseURL  string
	AppBaseURL    string
	ExportBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mixpanel subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flag, bad enum,
// unknown subcommand) are exit 2; runtime/API errors (Mixpanel non-2xx,
// transport failure) and missing/invalid credentials are exit 1. Errors render
// to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	c, err := s.buildClient(env)
	if err != nil {
		// Credential/config checks run before cobra parses flags, so detect
		// --json in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(c)
	root.SetArgs(args)
	execErr := root.ExecuteContext(ctx)
	if execErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, execErr)

	var apiErr *apiError
	if errors.As(execErr, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause (401/403).
		return execution.Failure(execErr), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// buildClient parses the packed credential payload and resolves the region
// hosts. A missing/malformed payload, a missing username/secret/project_id, or
// an invalid region is a fatal config error (exit 1).
func (s *Service) buildClient(env map[string]string) (*client, error) {
	raw := env[EnvCredentials]
	if raw == "" {
		return nil, &usageError{msg: EnvCredentials + " is not set"}
	}
	var cr credentials
	if err := json.Unmarshal([]byte(raw), &cr); err != nil {
		return nil, &usageError{msg: EnvCredentials + " is not valid JSON"}
	}
	if cr.Username == "" {
		return nil, &usageError{msg: EnvCredentials + " is missing \"username\""}
	}
	if cr.Secret == "" {
		return nil, &usageError{msg: EnvCredentials + " is missing \"secret\""}
	}
	if cr.ProjectID == "" {
		return nil, &usageError{msg: EnvCredentials + " is missing \"project_id\""}
	}
	h, err := resolveHosts(cr.Region)
	if err != nil {
		return nil, &usageError{msg: err.Error()}
	}
	c := &client{
		hc:         s.HC,
		authHeader: basicAuth(cr.Username, cr.Secret),
		projectID:  cr.ProjectID,
		queryBase:  firstNonEmpty(s.QueryBaseURL, h.query),
		appBase:    firstNonEmpty(s.AppBaseURL, h.app),
		exportBase: firstNonEmpty(s.ExportBaseURL, h.export),
		out:        s.stdout(),
		err:        s.stderr(),
	}
	return c, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// credential/config check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api|credential|rateLimit",
// "status":<HTTP or omitted>,"retry_after_seconds":<omitted unless rateLimit>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		kind := apiErr.kind
		if kind == "" {
			kind = "api"
		}
		payload["kind"] = kind
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if apiErr.retryAfter > 0 {
			payload["retry_after_seconds"] = apiErr.retryAfter
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

// newRoot builds the resource-grouped cobra tree. Cross-resource verbs
// (segmentation, events, events-names, retention, retention-frequency,
// insights, engage, export, me) are top-level; funnels/cohorts/lexicon hang
// under a runnable group.
func (s *Service) newRoot(c *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "mixpanel",
		Short:         "Mixpanel product-analytics reads (Query, Lexicon, Export, App)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(c.out)
	root.SetErr(c.err)
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	funnels := newGroupCmd("funnels", "Saved funnels")
	funnels.AddCommand(s.newFunnelsListCmd(c), s.newFunnelsRunCmd(c))

	cohorts := newGroupCmd("cohorts", "Saved cohorts")
	cohorts.AddCommand(s.newCohortsListCmd(c))

	lexicon := newGroupCmd("lexicon", "Lexicon schemas (authored definitions only)")
	lexicon.AddCommand(s.newLexiconListCmd(c))

	root.AddCommand(
		s.newSegmentationCmd(c),
		s.newEventsCmd(c),
		s.newEventsNamesCmd(c),
		s.newRetentionCmd(c),
		s.newRetentionFrequencyCmd(c),
		s.newInsightsCmd(c),
		s.newEngageCmd(c),
		s.newExportCmd(c),
		s.newMeCmd(c),
		funnels, cohorts, lexicon,
	)
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
