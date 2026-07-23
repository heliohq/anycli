// Package sendgrid is the built-in SendGrid service: a non-interactive cobra
// tree over the Twilio SendGrid v3 REST surface (https://api.sendgrid.com/v3).
// Auth is "Authorization: Bearer <API_KEY>" on every request. SendGrid errors
// are non-2xx with a JSON body carrying {"errors":[{"field","message"}]}; only
// a 401 rejects the credential (a 403 is a normal scope/verified-sender error,
// not a dead key). Every command emits the provider JSON on stdout (passthrough
// + newline) except `mail send`, whose provider response is an empty 202 body,
// so it synthesizes {"status":"accepted","message_id":"<X-Message-Id>"}.
package sendgrid

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (GET / search lookups), "true" for
// provider-state mutations (send / upsert).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// DefaultBaseURL is the production SendGrid v3 API base (global host).
const DefaultBaseURL = "https://api.sendgrid.com/v3"

// EUBaseURL is the SendGrid v3 API base for the EU data-residency region. A
// key bound to an EU subuser must call this host; calling the global host
// routes data globally (see DESIGN §1 host divergence).
const EUBaseURL = "https://api.eu.sendgrid.com/v3"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/sendgrid.json). SendGrid API keys are long-lived, non-
// expiring bearer tokens minted in the SendGrid app.
const EnvAPIKey = "SENDGRID_API_KEY"

// EnvRegion optionally selects the data-residency host without a flag; the
// --region flag overrides it. Values: "global" (default) or "eu".
const EnvRegion = "SENDGRID_REGION"

const (
	regionGlobal = "global"
	regionEU     = "eu"
)

// Service implements the built-in SendGrid tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the SendGrid API base; empty = region-selected host.
	// Tests point it at an httptest server (which then wins over --region).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one sendgrid subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIKey]
	if token == "" {
		fmt.Fprintln(s.stderr(), "SENDGRID_API_KEY is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	defaultRegion := strings.ToLower(strings.TrimSpace(env[EnvRegion]))
	if defaultRegion == "" {
		defaultRegion = regionGlobal
	}
	root := s.newRoot(token, defaultRegion)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(token, defaultRegion string) *cobra.Command {
	region := new(string)
	root := &cobra.Command{
		Use:           "sendgrid",
		Short:         "SendGrid built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")
	root.PersistentFlags().StringVar(region, "region", defaultRegion, "data-residency region: global|eu")

	root.AddCommand(
		s.newScopesCmd(token, region),
		s.newMailCmd(token, region),
		s.newTemplateCmd(token, region),
		s.newContactCmd(token, region),
		s.newListCmd(token, region),
		s.newSuppressionCmd(token, region),
		s.newStatsCmd(token, region),
		s.newSenderCmd(token, region),
	)
	return root
}

// baseURL resolves the effective API base. An explicit BaseURL (tests) always
// wins; otherwise the region selects global vs EU host.
func (s *Service) baseURL(region string) string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	if strings.EqualFold(strings.TrimSpace(region), regionEU) {
		return EUBaseURL
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
