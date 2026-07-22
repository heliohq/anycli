// Package mailjet is the built-in Mailjet service: a non-interactive cobra tree
// over the Mailjet Email API. Transactional sends go through the Send API v3.1
// (POST /v3.1/send); everything else uses the v3 REST surface
// (/v3/REST/<resource>), which returns a {"Count","Data":[…],"Total"} envelope
// this service unwraps so callers never see the wrapper.
//
// Auth is HTTP Basic: the injected MAILJET_BASIC_AUTH credential is the exact
// Basic userinfo string "<api_key>:<secret_key>" (Mailjet's own cURL shape,
// --user "$MJ_APIKEY_PUBLIC:$MJ_APIKEY_PRIVATE"), which this service base64-
// encodes into "Authorization: Basic …" on every request. A 401/403 rejects
// the credential.
//
// The API host defaults to https://api.mailjet.com; accounts provisioned on
// Mailjet's US architecture use https://api.us.mailjet.com, selectable with
// --region us or an explicit --base-url. The credential carries no host marker,
// so the same key:secret works against whichever host the account lives on.
package mailjet

import (
	"context"
	"encoding/base64"
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

// DefaultBaseURL is the default (EU-architecture) Mailjet API host. Paths carry
// their own version prefix (/v3/REST/… or /v3.1/…), so the base is host-only.
const DefaultBaseURL = "https://api.mailjet.com"

// usBaseURL is the documented host for accounts on Mailjet's US architecture
// (official .NET/PHP wrapper READMEs). Selected with --region us.
const usBaseURL = "https://api.us.mailjet.com"

// EnvBasicAuth is the env var the credential binding injects
// (definitions/tools/mailjet.json). Its value is the Basic userinfo string
// "<api_key>:<secret_key>"; this service base64-encodes it per request.
const EnvBasicAuth = "MAILJET_BASIC_AUTH"

// Service implements the built-in Mailjet tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API host; empty = DefaultBaseURL. Tests point it at
	// an httptest server. A --base-url / --region flag also feeds this at
	// runtime (flag value wins over this field when set).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mailjet subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, invalid JSON,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Mailjet non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	basic := strings.TrimSpace(env[EnvBasicAuth])
	if basic == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvBasicAuth + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(basic)
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

func (s *Service) newRoot(basic string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mailjet",
		Short:         "Mailjet built-in service (transactional email, contacts, lists, templates, stats)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "output JSON (always on; accepted for uniformity)")
	pf.String("base-url", "", "override the API host (default https://api.mailjet.com)")
	pf.String("region", "", "API region: eu (default) or us (https://api.us.mailjet.com)")

	root.AddCommand(
		s.newSendCmd(basic),
		s.newContactCmd(basic),
		s.newListCmd(basic),
		s.newTemplateCmd(basic),
		s.newMessageCmd(basic),
		s.newStatCmd(basic),
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails with exit 2.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// resolveBaseURL resolves the API host from --base-url / --region flags, the
// Service.BaseURL field (tests / httptest), then the default. --base-url wins
// over --region; the two flags are mutually exclusive when both are set to
// conflicting values.
func (s *Service) resolveBaseURL(cmd *cobra.Command) (string, error) {
	base, _ := cmd.Flags().GetString("base-url")
	region, _ := cmd.Flags().GetString("region")
	if base != "" {
		return strings.TrimRight(base, "/"), nil
	}
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "", "eu":
		if s.BaseURL != "" {
			return strings.TrimRight(s.BaseURL, "/"), nil
		}
		return DefaultBaseURL, nil
	case "us":
		return usBaseURL, nil
	default:
		return "", &usageError{msg: fmt.Sprintf("--region %q is not valid (use eu or us)", region)}
	}
}

// basicAuthHeader returns the "Basic <base64(api_key:secret_key)>" value from
// the injected userinfo string. Single source of the Basic-auth encoding.
func basicAuthHeader(userinfo string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(userinfo))
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse missing-credential check).
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
