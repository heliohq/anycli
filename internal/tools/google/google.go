// Package google is the built-in Google Workspace service: a non-interactive
// cobra tree over the Gmail / Calendar / Drive REST APIs with one user access
// token. A 401/403 very often means the token lacks a scope the user never
// granted — those errors carry an explicit reconnect hint.
package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// Production API bases, one per Google product surface. Each is independently
// injectable for tests.
const (
	DefaultGmailBase = "https://gmail.googleapis.com/gmail/v1"
	DefaultCalBase   = "https://www.googleapis.com/calendar/v3"
	DefaultDriveBase = "https://www.googleapis.com/drive/v3"
)

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/google.json).
const EnvAccessToken = "GOOGLE_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: with OAuth incremental consent the
// usual cause is a scope the user did not grant on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Google Workspace tool. It satisfies
// tools.Service by duck typing (no registry import — no import cycle).
type Service struct {
	// GmailBase / CalBase / DriveBase override the per-product API bases;
	// empty = the production defaults. Tests point them at httptest servers.
	GmailBase string
	CalBase   string
	DriveBase string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one google subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "GOOGLE_ACCESS_TOKEN is not set")
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

func (s *Service) gmailBase() string {
	if s.GmailBase != "" {
		return s.GmailBase
	}
	return DefaultGmailBase
}

func (s *Service) calBase() string {
	if s.CalBase != "" {
		return s.CalBase
	}
	return DefaultCalBase
}

func (s *Service) driveBase() string {
	if s.DriveBase != "" {
		return s.DriveBase
	}
	return DefaultDriveBase
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "google",
		Short:         "Google Workspace built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	gmail := &cobra.Command{Use: "gmail", Short: "Gmail"}
	gmail.AddCommand(s.newGmailSendCmd(token), s.newGmailListCmd(token))

	calendar := &cobra.Command{Use: "calendar", Short: "Calendar"}
	calendar.AddCommand(s.newCalendarListCmd(token), s.newCalendarCreateCmd(token))

	drive := &cobra.Command{Use: "drive", Short: "Drive"}
	drive.AddCommand(s.newDriveListCmd(token))

	root.AddCommand(gmail, calendar, drive)
	return root
}

func (s *Service) newGmailSendCmd(token string) *cobra.Command {
	var to, subject, body string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an email",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw := rfc822Message(to, subject, body)
			payload := map[string]string{"raw": base64.URLEncoding.EncodeToString([]byte(raw))}
			respBody, err := s.call(cmd.Context(), token, http.MethodPost, s.gmailBase()+"/users/me/messages/send", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(respBody)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "recipient address")
	cmd.Flags().StringVar(&subject, "subject", "", "subject line")
	cmd.Flags().StringVar(&body, "body", "", "plain-text body")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newGmailListCmd(token string) *cobra.Command {
	var query string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if query != "" {
				q.Set("q", query)
			}
			q.Set("maxResults", strconv.Itoa(limit))
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.gmailBase()+"/users/me/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Gmail search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "max messages to return")
	return cmd
}

func (s *Service) newCalendarListCmd(token string) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List upcoming events on the primary calendar",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now()
			q := url.Values{}
			q.Set("timeMin", now.Format(time.RFC3339))
			q.Set("timeMax", now.AddDate(0, 0, days).Format(time.RFC3339))
			q.Set("singleEvents", "true")
			q.Set("orderBy", "startTime")
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.calBase()+"/calendars/primary/events", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "look-ahead window in days")
	return cmd
}

func (s *Service) newCalendarCreateCmd(token string) *cobra.Command {
	var title, start, end, attendees string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an event on the primary calendar",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{
				"summary": title,
				"start":   map[string]string{"dateTime": start},
				"end":     map[string]string{"dateTime": end},
			}
			if attendees != "" {
				var list []map[string]string
				for _, addr := range strings.Split(attendees, ",") {
					if addr = strings.TrimSpace(addr); addr != "" {
						list = append(list, map[string]string{"email": addr})
					}
				}
				payload["attendees"] = list
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, s.calBase()+"/calendars/primary/events", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "event title")
	cmd.Flags().StringVar(&start, "start", "", "start time (RFC3339)")
	cmd.Flags().StringVar(&end, "end", "", "end time (RFC3339)")
	cmd.Flags().StringVar(&attendees, "attendees", "", "comma-separated attendee emails")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	return cmd
}

func (s *Service) newDriveListCmd(token string) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if query != "" {
				q.Set("q", query)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.driveBase()+"/files", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Drive search query")
	return cmd
}

// rfc822Message builds the minimal RFC 822 message Gmail's send API expects.
func rfc822Message(to, subject, body string) string {
	return "To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		body
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Google API request with Bearer auth. Non-2xx surfaces the
// body's error message; 401/403 additionally carry the missing-scope hint.
func (s *Service) call(ctx context.Context, token, method, endpoint string, query url.Values, payload any) ([]byte, error) {
	u := endpoint
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("google: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			hint = scopeHint
		}
		apiErr := fmt.Errorf("google API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), hint)
		return nil, classifyGoogleCredentialError(resp.StatusCode, body, apiErr)
	}
	return body, nil
}

// apiMessage extracts Google's error message from an error body, falling back
// to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.Status != "" || e.Error.Message != "") {
		return strings.TrimSpace(strings.TrimPrefix(e.Error.Status+": "+e.Error.Message, ": "))
	}
	return string(body)
}
