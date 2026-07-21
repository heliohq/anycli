// Package linkedin is the built-in LinkedIn service: a non-interactive cobra
// tree over the api.linkedin.com surface. Posting goes through the versioned
// REST API (/rest/posts) which requires the pinned LinkedIn-Version and the
// Restli protocol header; identity comes from the OIDC /v2/userinfo endpoint.
package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultAPIBase is the production LinkedIn API base.
const DefaultAPIBase = "https://api.linkedin.com"

// linkedinVersion pins the LinkedIn-Version header sent on /rest/* calls.
const linkedinVersion = "202406"

// restliProtocolVersion is required by the versioned REST API.
const restliProtocolVersion = "2.0.0"

// Env vars the credential bindings inject (definitions/tools/linkedin.json).
// PersonURN is captured server-side at connect time; it can legitimately be
// missing (best-effort capture) — posting then fails with a reconnect hint.
const (
	EnvAccessToken = "LINKEDIN_ACCESS_TOKEN"
	EnvPersonURN   = "LINKEDIN_PERSON_URN"
)

// Service implements the built-in LinkedIn tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// APIBase overrides the LinkedIn API base; empty = DefaultAPIBase. Tests
	// point it at an httptest server.
	APIBase string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one linkedin subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "LINKEDIN_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token, env[EnvPersonURN])
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
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

func (s *Service) newRoot(token, personURN string) *cobra.Command {
	root := &cobra.Command{
		Use:           "linkedin",
		Short:         "LinkedIn built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	post := &cobra.Command{Use: "post", Short: "Posts"}
	post.AddCommand(s.newPostCreateCmd(token, personURN))

	root.AddCommand(post, s.newMeCmd(token))
	return root
}

func (s *Service) newPostCreateCmd(token, personURN string) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Publish a post as the connected member",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /rest/posts
		RunE: func(cmd *cobra.Command, _ []string) error {
			if personURN == "" {
				return fmt.Errorf("person_urn missing — reconnect LinkedIn to capture it")
			}
			payload := map[string]any{
				"author":         personURN,
				"commentary":     text,
				"visibility":     "PUBLIC",
				"distribution":   map[string]any{"feedDistribution": "MAIN_FEED"},
				"lifecycleState": "PUBLISHED",
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/rest/posts", true, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "post text")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Show the connected member's identity (OIDC userinfo)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET /v2/userinfo
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/userinfo", false, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// emit writes the provider's JSON response to stdout verbatim. /rest/posts
// answers 201 with an empty body — emit an empty line in that case.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one LinkedIn API request with Bearer auth. versioned adds the
// pinned LinkedIn-Version + Restli protocol headers required by /rest/*.
// Non-2xx surfaces the body's message.
func (s *Service) call(ctx context.Context, token, method, path string, versioned bool, payload any) ([]byte, error) {
	base := s.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("linkedin: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if versioned {
		req.Header.Set("LinkedIn-Version", linkedinVersion)
		req.Header.Set("X-Restli-Protocol-Version", restliProtocolVersion)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("linkedin: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("linkedin API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// apiMessage extracts LinkedIn's error message from an error body, falling
// back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message        string `json:"message"`
		ServiceErrCode int    `json:"serviceErrorCode"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return e.Message
	}
	return string(body)
}

// NewCommandTree returns the full command tree built with empty credentials
// for dry-run parsing and traversal (tools.Service seam, design 318). The
// credentials are only captured by RunE closures, which are never run on
// this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("", "") }
