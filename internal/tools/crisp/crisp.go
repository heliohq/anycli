// Package crisp is the built-in Crisp service: a non-interactive cobra tree over
// the Crisp REST API v1 (https://api.crisp.chat/v1). It wraps the website-scoped
// inbox surface a support teammate reaches for — conversations, messages, and
// People contacts — all under /v1/website/{website_id}/.
//
// Auth is an HTTP Basic keypair: CRISP_TOKEN carries the "identifier:key" pair
// (Crisp's own `curl --user "{id}:{key}"` shape), sent as
// "Authorization: Basic base64(identifier:key)" plus the constant
// "X-Crisp-Tier: website" header. website_id is a routing parameter, not a
// credential: every verb requires --website. A website-tier token cannot
// enumerate websites, so there is no auto-resolve fallback.
//
// Crisp wraps every response in a {error, reason, data} envelope. Success emits
// {"data": <crisp data>, "meta": {...}} on stdout; failures (Crisp error:true or
// a non-2xx status) render {"error": {message, kind, status}} on stderr with a
// non-zero exit. Exit codes: 0 success, 1 runtime/API failure (typed apiError),
// 2 usage/parse errors (including a missing --website). A 401 or invalid_session
// marks the credential rejected.
package crisp

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

// DefaultBaseURL is the production Crisp REST API v1 base.
const DefaultBaseURL = "https://api.crisp.chat/v1"

// tierWebsite is the fixed X-Crisp-Tier header value v1 sends on every request.
// v1 connects with an owner-generated website token; the plugin tier is a
// deferred future option (see the provider DESIGN).
const tierWebsite = "website"

// EnvToken is the env var the credential binding injects
// (definitions/tools/crisp.json). It carries the "identifier:key" keypair.
const EnvToken = "CRISP_TOKEN"

// Service implements the built-in Crisp tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Crisp API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one crisp subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: "CRISP_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if !strings.Contains(token, ":") {
		s.renderError(hasJSONArg(args), &usageError{msg: "CRISP_TOKEN must be in identifier:key form"})
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
		return execution.Failure(err), nil
	}
	// usageError plus every cobra parse/arg/enum/unknown-command error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse token checks).
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

// newRoot builds the grouped-by-resource cobra tree. --website is a global
// persistent flag; each leaf validates it via websiteFlag (missing → exit 2).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "crisp",
		Short:         "Crisp built-in service (website inbox + contacts)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "output JSON (always on; accepted for uniformity)")
	pf.String("website", "", "Crisp website_id (required; find it in your Crisp dashboard URL)")

	root.AddCommand(
		s.newConversationCmd(token),
		s.newPeopleCmd(token),
	)
	return root
}

// authHeader returns the HTTP Basic value for a Crisp keypair. The token is
// already the "identifier:key" userinfo string, so base64 of the whole token IS
// the Basic credential — no split is needed at this layer.
func authHeader(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
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
