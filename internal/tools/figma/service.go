// Package figma implements the built-in, non-interactive Figma REST service.
package figma

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

const (
	// DefaultBaseURL is the production Figma REST API base.
	DefaultBaseURL = "https://api.figma.com"
	// EnvAccessToken is populated by definitions/tools/figma.json.
	EnvAccessToken = "FIGMA_ACCESS_TOKEN"
)

// Service executes Figma REST commands. Tests override the API base, HTTP
// client, and output streams; zero values target production.
type Service struct {
	BaseURL string
	HC      *http.Client
	Out     io.Writer
	Err     io.Writer
}

// Execute runs one Figma command with a resolver-injected personal access token.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "FIGMA_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), redactSecret(err.Error(), token))
		return execution.Result{ExitCode: 1, CredentialRejected: isCredentialRejected(err)}, nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "figma",
		Short:         "Figma REST API",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newAPICommand(token),
		s.newMeCommand(token),
		s.newTeamsCommand(token),
		s.newProjectsCommand(token),
		s.newFilesCommand(token),
		s.newImagesCommand(token),
		s.newCommentsCommand(token),
		s.newLibrariesCommand(token),
		s.newVariablesCommand(token),
		s.newDevResourcesCommand(token),
		s.newWebhooksCommand(token),
		s.newAnalyticsCommand(token),
		s.newOEmbedCommand(token),
		s.newPaymentsCommand(token),
		s.newContextCommand(token),
		s.newAssetsCommand(token),
		s.newCapabilitiesCommand(),
	)
	return root
}

func (s *Service) newMeCommand(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated Figma user",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getMe",
			sideEffectAnnotation:  "false", // GET /v1/me
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getMe", nil, nil)
		},
	}
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

func setOptionalString(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setOptionalPositiveInt(query url.Values, key string, value int) error {
	if value < 0 {
		return fmt.Errorf("--%s must be positive", key)
	}
	if value > 0 {
		query.Set(key, strconv.Itoa(value))
	}
	return nil
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
