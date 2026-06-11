// Package slack is the built-in Slack service: a non-interactive cobra tree
// over the api.slack.com REST surface. Slack's dialect returns HTTP 200 with
// {"ok":false,"error":...} on failure — EVERY call checks it; an HTTP status
// alone proves nothing.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Slack Web API base.
const DefaultBaseURL = "https://api.slack.com/api"

// EnvBotToken is the env var the credential binding injects (definitions/tools/slack.json).
const EnvBotToken = "SLACK_BOT_TOKEN"

// Service implements the built-in Slack tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Slack API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one slack subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (int, error) {
	token := env[EnvBotToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "SLACK_BOT_TOKEN is not set")
		return 1, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return 1, nil
	}
	return 0, nil
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

// newRoot builds the cobra tree for one invocation. Non-interactive by design:
// all input comes from flags; output is the provider's JSON on stdout.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "slack",
		Short:         "Slack built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	// --json is accepted for a uniform agent-facing surface; JSON is already
	// the only output format.
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	chat := &cobra.Command{Use: "chat", Short: "Messages"}
	chat.AddCommand(s.newChatPostCmd(token), s.newChatHistoryCmd(token))

	channels := &cobra.Command{Use: "channels", Short: "Channels"}
	channels.AddCommand(s.newChannelsListCmd(token))

	root.AddCommand(chat, channels)
	return root
}

func (s *Service) newChatPostCmd(token string) *cobra.Command {
	var channel, text string
	cmd := &cobra.Command{
		Use:   "post",
		Short: "Post a message to a channel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/chat.postMessage", nil,
				map[string]string{"channel": channel, "text": text})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel id or name")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	_ = cmd.MarkFlagRequired("channel")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func (s *Service) newChatHistoryCmd(token string) *cobra.Command {
	var channel string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Fetch a channel's message history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("channel", channel)
			q.Set("limit", strconv.Itoa(limit))
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations.history", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel id")
	cmd.Flags().IntVar(&limit, "limit", 20, "max messages to return")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}

func (s *Service) newChannelsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List channels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("types", "public_channel,private_channel")
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations.list", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Slack Web API request and enforces the Slack dialect:
// any response (HTTP 200 included) with ok != true is an error carrying the
// Slack error code.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("slack: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("slack: read response: %w", err)
	}
	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("slack: invalid response (HTTP %d): %w", resp.StatusCode, err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("slack API error: %s (HTTP %d)", envelope.Error, resp.StatusCode)
	}
	return body, nil
}
