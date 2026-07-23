// Package braintree is the built-in Braintree service: a cobra tree over the
// Braintree GraphQL API (payments.[sandbox.]braintree-api.com/graphql). It
// covers a merchant's operational surface — transaction search/inspect,
// refund/void/reverse, customer and dispute lookup, subscription status, plus a
// read-only raw `query` escape hatch. Charge-creation is deliberately out of
// scope (it needs a client-collected paymentMethodId this server-side tool does
// not hold).
//
// Auth is HTTP Basic base64(public_key:private_key) with a pinned
// Braintree-Version date header; the host is selected by the injected
// BRAINTREE_ENVIRONMENT (sandbox|production). GraphQL errors return HTTP 200
// with a top-level errors[] array, so success is body-shaped, not
// status-shaped (see client.go).
package braintree

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

// Service implements the built-in Braintree tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the environment-derived GraphQL host; empty = derive
	// from BRAINTREE_ENVIRONMENT. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one braintree subcommand with the resolved credentials in env.
// Exit codes: 0 success; 2 usage/parse (bad flags, a mutation supplied to the
// read-only `query` passthrough); 1 runtime/API failure (GraphQL errors[],
// non-2xx, transport, bad environment).
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	jsonMode := hasJSONArg(args)

	publicKey := env[EnvPublicKey]
	privateKey := env[EnvPrivateKey]
	if publicKey == "" || privateKey == "" {
		s.renderError(jsonMode, &apiError{msg: "BRAINTREE_PUBLIC_KEY and BRAINTREE_PRIVATE_KEY must be set"})
		return execution.Result{ExitCode: 1}, nil
	}

	baseURL := s.BaseURL
	if baseURL == "" {
		resolved, err := resolveBaseURL(env[EnvEnvironment])
		if err != nil {
			s.renderError(jsonMode, err)
			return execution.Result{ExitCode: 1}, nil
		}
		baseURL = resolved
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	cl := &client{
		baseURL:    baseURL,
		authHeader: basicAuthHeader(publicKey, privateKey),
		hc:         hc,
	}

	root := s.newRoot(cl)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	mode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(mode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) || execution.IsCredentialRejected(err) {
		return execution.Failure(err), nil
	}
	// usageError plus every cobra parse/arg/unknown-command error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"class":…}} at exit 1 (notion's structured envelope);
// the "class" is present only for GraphQL apiErrors that carry an errorClass.
// Secrets never reach this path — apiError messages are provider text, never
// the key pair.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.class != "" {
		payload["class"] = apiErr.class
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

// newRoot builds the resource-grouped cobra tree. transaction / customer /
// dispute / subscription are groups; ping and query are top-level leaves.
func (s *Service) newRoot(cl *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "braintree",
		Short:         "Braintree payments operations via the GraphQL API",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "emit compact structured JSON (default is indented JSON)")

	root.AddCommand(
		s.newPingCmd(cl),
		s.newTransactionCmd(cl),
		s.newCustomerCmd(cl),
		s.newDisputeCmd(cl),
		s.newSubscriptionCmd(cl),
		s.newQueryCmd(cl),
	)
	return root
}

// newGroupCmd is a runnable, help-only command group. cobra skips Args
// validation on non-runnable commands (help + exit 0 even for an unknown
// subcommand — a false success for an agent); making the group help-only
// restores it and satisfies the design-318 lint (group RunE must be help-only).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with a nil client for
// dry-run parsing and traversal (tools.Service seam, design 318). The client is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(nil) }

// jsonModeFrom reads the persistent --json flag off any command in the tree.
func jsonModeFrom(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// emit writes a normalized result object to stdout: compact JSON under --json,
// indented JSON otherwise. Both are structured and secret-free.
func (s *Service) emit(cmd *cobra.Command, value any) error {
	var (
		b   []byte
		err error
	)
	if jsonModeFrom(cmd) {
		b, err = json.Marshal(value)
	} else {
		b, err = json.MarshalIndent(value, "", "  ")
	}
	if err != nil {
		return &apiError{msg: fmt.Sprintf("braintree: encode output: %v", err), err: err}
	}
	_, werr := s.stdout().Write(append(b, '\n'))
	return werr
}
