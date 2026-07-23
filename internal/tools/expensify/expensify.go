// Package expensify is the built-in Expensify service: a non-interactive cobra
// tree over the Expensify Integration Server API
// (https://integrations.expensify.com/Integration-Server/ExpensifyIntegrations).
//
// Expensify has a single endpoint: every job — read or write — is a POST whose
// form field `requestJobDescription` carries a JSON document
// {type, credentials, inputSettings, …}. Auth is a partnerUserID +
// partnerUserSecret PAIR. Helio stores that pair as one opaque secret
// "partnerUserID:partnerUserSecret" (design note: colon-pair single secret) and
// injects it as EXPENSIFY_CREDENTIALS; this service splits it on the first colon
// and builds the credentials object. Responses are HTTP 200 with a JSON body
// carrying responseCode (401/403 → credential rejected); some jobs return a
// plain body (e.g. a generated filename), which is emitted verbatim.
package expensify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Expensify Integration Server endpoint.
const DefaultBaseURL = "https://integrations.expensify.com/Integration-Server/ExpensifyIntegrations"

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/expensify.json). Its value is the opaque pair
// "partnerUserID:partnerUserSecret"; the two halves are split on the first colon.
const EnvCredentials = "EXPENSIFY_CREDENTIALS"

// credentials is the Integration Server auth object sent on every job.
type credentials struct {
	PartnerUserID     string `json:"partnerUserID"`
	PartnerUserSecret string `json:"partnerUserSecret"`
}

// Service implements the built-in Expensify tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Integration Server endpoint; empty = DefaultBaseURL.
	// Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// usageError marks a locally-detected input problem (bad flag combo, malformed
// --input JSON) — exit 2, never a provider fault.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Execute runs one expensify subcommand with the resolved credential pair in env.
// Missing/malformed credentials are exit 1; provider/runtime failures are exit 1
// (apiError); usage/param errors are exit 2 (every cobra-originated or
// usageError result).
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	raw := env[EnvCredentials]
	if strings.TrimSpace(raw) == "" {
		fmt.Fprintf(s.stderr(), "%s is not set\n", EnvCredentials)
		return execution.Result{ExitCode: 1}, nil
	}
	creds, err := parseCredentials(raw)
	if err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(creds)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return execution.Failure(err), nil
		}
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
}

// parseCredentials splits the injected pair on the FIRST colon: partnerUserID is
// colon-free (a token/email) and partnerUserSecret is alphanumeric, so a first
// split is lossless even if the secret ever carried a colon.
func parseCredentials(raw string) (credentials, error) {
	i := strings.IndexByte(raw, ':')
	if i < 0 {
		return credentials{}, fmt.Errorf("%s must be in the form partnerUserID:partnerUserSecret", EnvCredentials)
	}
	id := strings.TrimSpace(raw[:i])
	secret := strings.TrimSpace(raw[i+1:])
	if id == "" || secret == "" {
		return credentials{}, fmt.Errorf("%s must be in the form partnerUserID:partnerUserSecret", EnvCredentials)
	}
	return credentials{PartnerUserID: id, PartnerUserSecret: secret}, nil
}

func (s *Service) newRoot(creds credentials) *cobra.Command {
	root := &cobra.Command{
		Use:           "expensify",
		Short:         "Expensify built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")
	root.AddCommand(
		s.newPolicyCmd(creds),
		s.newRequestCmd(creds),
	)
	return root
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return s.BaseURL
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

// NewCommandTree returns the full command tree built with empty credentials for
// dry-run parsing and traversal (tools.Service seam, design 318). The credential
// pair is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(credentials{}) }
