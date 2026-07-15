// Package discord is the built-in Discord service: a non-interactive cobra
// tree over the discord.com/api/v10 REST surface. Discord bot auth uses
// "Authorization: Bot <token>" — NOT Bearer; failures are non-2xx with a JSON
// body carrying code/message.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Discord API base.
const DefaultBaseURL = "https://discord.com/api/v10"

// EnvBotToken is the env var the credential binding injects
// (definitions/tools/discord.json). The resolved access_token is the bot token
// in v1 single-app mode (Helio design 227 D6).
const EnvBotToken = "DISCORD_BOT_TOKEN"

// Service implements the built-in Discord tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Discord API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one discord subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvBotToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "DISCORD_BOT_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "discord",
		Short:         "Discord built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	message := &cobra.Command{Use: "message", Short: "Messages"}
	message.AddCommand(s.newMessageSendCmd(token))

	channels := &cobra.Command{Use: "channels", Short: "Channels"}
	channels.AddCommand(s.newChannelsListCmd(token))

	root.AddCommand(message, channels)
	return root
}

func (s *Service) newMessageSendCmd(token string) *cobra.Command {
	var channel, text string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a channel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/channels/"+url.PathEscape(channel)+"/messages",
				map[string]string{"content": text})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel id")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	_ = cmd.MarkFlagRequired("channel")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func (s *Service) newChannelsListCmd(token string) *cobra.Command {
	var guild string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a guild's channels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/guilds/"+url.PathEscape(guild)+"/channels", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&guild, "guild", "", "guild (server) id")
	_ = cmd.MarkFlagRequired("guild")
	return cmd
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Discord API request with bot auth ("Bot <token>").
// Non-2xx surfaces the body's code/message.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("discord: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("discord: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("discord API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// apiMessage extracts Discord's error message (code + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    json.Number `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Code != "" {
			return fmt.Sprintf("%s (code %s)", e.Message, e.Code)
		}
		return e.Message
	}
	return string(body)
}
