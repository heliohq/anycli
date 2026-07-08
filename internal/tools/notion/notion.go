// Package notion is the built-in Notion service: a non-interactive cobra tree
// over the api.notion.com REST surface. Notion fails with a non-2xx status and
// a JSON body carrying code/message — every call surfaces both.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Notion API base.
const DefaultBaseURL = "https://api.notion.com/v1"

// notionVersion is the default Notion-Version header; commands can override it
// per call via callWithVersion (page read uses markdownVersion).
const notionVersion = "2022-06-28"

// markdownVersion is the Notion-Version required by the page-markdown
// endpoints; page read uses it without moving the global pin.
const markdownVersion = "2026-03-11"

// EnvToken is the env var the credential binding injects (definitions/tools/notion.json).
const EnvToken = "NOTION_TOKEN"

// Service implements the built-in Notion tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Notion API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one notion subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (int, error) {
	token := env[EnvToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "NOTION_TOKEN is not set")
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "notion",
		Short:         "Notion built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	page := &cobra.Command{Use: "page", Short: "Pages"}
	page.AddCommand(s.newPageCreateCmd(token), s.newPageGetCmd(token), s.newPageAppendCmd(token), s.newPageReadCmd(token))

	db := &cobra.Command{Use: "db", Short: "Databases"}
	db.AddCommand(s.newDBQueryCmd(token))

	root.AddCommand(page, s.newSearchCmd(token), db)
	return root
}

func (s *Service) newPageCreateCmd(token string) *cobra.Command {
	var parent, title, content string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a page under a parent page",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{
				"parent": map[string]any{"page_id": parent},
				"properties": map[string]any{
					"title": map[string]any{
						"title": []any{textSpan(title)},
					},
				},
			}
			if content != "" {
				payload["children"] = []any{paragraphBlock(content)}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/pages", payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent page id")
	cmd.Flags().StringVar(&title, "title", "", "page title")
	cmd.Flags().StringVar(&content, "content", "", "initial content (one paragraph block)")
	_ = cmd.MarkFlagRequired("parent")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func (s *Service) newPageGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <page-id>",
		Short: "Fetch a page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/pages/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newPageReadCmd(token string) *cobra.Command {
	var includeTranscript bool
	cmd := &cobra.Command{
		Use:   "read <page-id>",
		Short: "Read a page's body as markdown",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/pages/" + url.PathEscape(args[0]) + "/markdown"
			if includeTranscript {
				path += "?" + url.Values{"include_transcript": {"true"}}.Encode()
			}
			body, err := s.callWithVersion(cmd.Context(), token, http.MethodGet, path, nil, markdownVersion)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().BoolVar(&includeTranscript, "include-transcript", false, "include meeting-note transcripts")
	return cmd
}

func (s *Service) newPageAppendCmd(token string) *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "append <page-id>",
		Short: "Append a paragraph block to a page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"children": []any{paragraphBlock(content)}}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/blocks/"+url.PathEscape(args[0])+"/children", payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "paragraph text to append")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search pages and databases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/search", map[string]any{"query": query})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func (s *Service) newDBQueryCmd(token string) *cobra.Command {
	var filterJSON string
	cmd := &cobra.Command{
		Use:   "query <database-id>",
		Short: "Query a database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if filterJSON != "" {
				var filter json.RawMessage
				if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
					return fmt.Errorf("--filter-json is not valid JSON: %w", err)
				}
				payload["filter"] = filter
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/databases/"+url.PathEscape(args[0])+"/query", payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&filterJSON, "filter-json", "", "raw Notion filter object (JSON)")
	return cmd
}

// textSpan builds one Notion rich-text span.
func textSpan(text string) map[string]any {
	return map[string]any{"type": "text", "text": map[string]any{"content": text}}
}

// paragraphBlock wraps text into one Notion paragraph block.
func paragraphBlock(text string) map[string]any {
	return map[string]any{
		"object": "block",
		"type":   "paragraph",
		"paragraph": map[string]any{
			"rich_text": []any{textSpan(text)},
		},
	}
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Notion API request on the globally pinned Notion-Version.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	return s.callWithVersion(ctx, token, method, path, payload, notionVersion)
}

// callWithVersion is call with a caller-chosen Notion-Version header. Bearer
// auth + the version on every call; non-2xx surfaces the body's message and,
// for 403/404, an actionable access hint.
func (s *Service) callWithVersion(ctx context.Context, token, method, path string, payload any, version string) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("notion: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("notion: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", version)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("notion: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("notion: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("notion API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
	}
	return body, nil
}

// accessHint returns an actionable clause for the failures an agent most often
// hits: a wrong ID or a resource never shared with the integration.
func accessHint(status int) string {
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return " (check the ID and that the integration has been granted access to this resource)"
	}
	return ""
}

// apiMessage extracts Notion's error message (code + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Code != "" || e.Message != "") {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return string(body)
}
