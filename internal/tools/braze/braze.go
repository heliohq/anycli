// Package braze is the built-in Braze service: a non-interactive cobra tree
// over the Braze REST surface (https://www.braze.com/docs/api). It reads
// campaign / Canvas / segment / KPI analytics, discovers content, triggers and
// schedules messages, and looks up / tracks users.
//
// Braze is a regional, multi-instance product: a workspace lives on exactly one
// cluster and its REST host differs per cluster, so the credential is a
// DSN-shaped secret that carries BOTH the REST API key (in userinfo) and the
// cluster host: https://<REST_API_KEY>@rest.iad-05.braze.com. The service
// reconstructs the Bearer key and the base URL from it (§credential). Auth is
// "Authorization: Bearer <key>" on every request. Braze fails with a non-2xx
// status and a JSON body carrying a message; 401 (bad/revoked key), 403 (key
// lacks the endpoint permission) and 429 (rate limited) are surfaced as
// distinct kinds so the host reacts differently to each.
package braze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/braze.json). It is the DSN-shaped secret
// https://<REST_API_KEY>@<cluster-rest-host>.
const EnvCredentials = "BRAZE_CREDENTIALS"

// Service implements the built-in Braze tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the reconstructed cluster base (tests point it at an
	// httptest server). The Bearer key is still taken from the DSN userinfo.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one braze subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid JSON, missing
// required flags, unknown subcommands, malformed/missing BRAZE_CREDENTIALS) are
// exit 2; runtime/API errors (Braze non-2xx, transport failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	apiKey, baseURL, err := parseCredentials(env[EnvCredentials])
	if err != nil {
		// Credential parsing runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: err.Error()})
		return execution.Result{ExitCode: 2}, nil
	}
	if s.BaseURL != "" {
		baseURL = strings.TrimRight(s.BaseURL, "/")
	}

	root := s.newRoot(apiKey, baseURL)
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
		// usageError plus every cobra parse/arg/unknown-command error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
}

// parseCredentials splits the DSN-shaped BRAZE_CREDENTIALS secret into the REST
// API key (userinfo) and the base URL (scheme://host, userinfo stripped). The
// host must be a known Braze cluster suffix so a mistyped cluster fails loudly
// at parse rather than as a silent 401. Errors are static guidance and never
// echo the secret.
func parseCredentials(raw string) (apiKey, baseURL string, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", fmt.Errorf("%s is not set; expected the DSN https://<REST_API_KEY>@<cluster-rest-host> (see https://www.braze.com/docs/api/basics)", EnvCredentials)
	}
	parsed, perr := url.Parse(value)
	if perr != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("%s is malformed; expected https://<REST_API_KEY>@<cluster-rest-host>", EnvCredentials)
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return "", "", fmt.Errorf("%s is missing the REST API key; expected https://<REST_API_KEY>@<cluster-rest-host>", EnvCredentials)
	}
	host := parsed.Host
	if !isBrazeHost(host) {
		return "", "", fmt.Errorf("%s host %q is not a Braze cluster host (expected *.braze.com or *.braze.eu; see the instance table at https://www.braze.com/docs/api/basics)", EnvCredentials, host)
	}
	return parsed.User.Username(), parsed.Scheme + "://" + host, nil
}

// isBrazeHost reports whether host is a Braze cluster REST host. Braze REST
// endpoints live under braze.com (US/AU/ID/JP/KR) and braze.eu (EU); anything
// else is a mistyped cluster.
func isBrazeHost(host string) bool {
	h := strings.ToLower(host)
	return strings.HasSuffix(h, ".braze.com") || strings.HasSuffix(h, ".braze.eu")
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":…,"status":…,"rate_limit_reset":…}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = apiErr.kind
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if apiErr.rateLimitReset != "" {
			payload["rate_limit_reset"] = apiErr.rateLimitReset
		}
		if apiErr.rateLimitRemaining != "" {
			payload["rate_limit_remaining"] = apiErr.rateLimitRemaining
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

// newRoot builds the grouped-by-resource cobra tree. Read/export/discovery
// groups (campaigns, canvas, segments, kpi, events, purchases, sessions,
// templates, content-blocks) plus the act groups (messages, subscription,
// users) each hang under a runnable resource group.
func (s *Service) newRoot(apiKey, baseURL string) *cobra.Command {
	c := &client{apiKey: apiKey, baseURL: baseURL, hc: s.HC, out: s.stdout()}

	root := &cobra.Command{
		Use:           "braze",
		Short:         "Braze customer-engagement built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newCampaignsCmd(c),
		s.newSendsCmd(c),
		s.newCanvasCmd(c),
		s.newSegmentsCmd(c),
		s.newKPICmd(c),
		s.newEventsCmd(c),
		s.newPurchasesCmd(c),
		s.newSessionsCmd(c),
		s.newTemplatesCmd(c),
		s.newContentBlocksCmd(c),
		s.newUsersCmd(c),
		s.newMessagesCmd(c),
		s.newSubscriptionCmd(c),
	)
	return root
}

// newGroupCmd is a runnable command group. A runnable group shows help on a
// bare invocation but still fails an unknown subcommand (cobra skips Args
// validation on non-runnable commands, which would exit 0 on a bad verb — a
// false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
