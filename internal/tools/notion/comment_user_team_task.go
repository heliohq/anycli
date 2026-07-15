package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newCommentCreateCmd is `comment create` (POST /v1/comments). Page-level only:
// the body is {"parent":{"page_id":…},"rich_text":[…]}. --content is plain text
// — this is the one write path that does not go through /markdown, and the
// comments endpoint takes structured rich_text, not a markdown string, so the
// content is dropped verbatim into a single text run with no markdown parsing
// (design 304 §comment). Output JSON.
func (s *Service) newCommentCreateCmd(token string) *cobra.Command {
	var pageID, content string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a page-level comment",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&pageID, "page-id", "", "page to comment on (required)")
	cmd.Flags().StringVar(&content, "content", "", "plain-text comment body, not markdown (required)")
	_ = cmd.MarkFlagRequired("page-id")
	_ = cmd.MarkFlagRequired("content")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		id, err := resolveID(pageID)
		if err != nil {
			return err
		}
		payload := map[string]any{
			"parent": map[string]any{"page_id": id},
			"rich_text": []any{
				map[string]any{"type": "text", "text": map[string]any{"content": content}},
			},
		}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/comments", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCommentListCmd is `comment list <page-id>` (GET /v1/comments?block_id=…).
// A page is itself a block, so the page id is passed as the block_id query
// param. Paginated like search / db query. Output JSON.
func (s *Service) newCommentListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <page-id>",
		Short: "List comments on a page",
		Args:  cobra.ExactArgs(1),
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		fetch := func(ctx context.Context, cursor string) ([]byte, error) {
			q := url.Values{}
			q.Set("block_id", id)
			if pf.pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pf.pageSize))
			}
			if cursor != "" {
				q.Set("start_cursor", cursor)
			}
			return s.call(ctx, token, http.MethodGet, "/comments?"+q.Encode(), nil)
		}
		body, err := paginate(cmd.Context(), pf.all, pf.startCursor, fetch)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newUserGetCmd is `user get [self] [--query <name|email>]` — three mutually
// exclusive forms: bare `self` → GET /v1/users/me; --query → list every user
// and keep case-insensitive substring matches on name OR email (0 hits is an
// empty list, not an error); no arg → GET /v1/users first page verbatim
// (has_more / next_cursor intact). self + --query together is a usage error.
// Output JSON.
func (s *Service) newUserGetCmd(token string) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "get [self]",
		Short: "Get the current user (self), search users (--query), or list all users",
		Args:  cobra.MaximumNArgs(1),
	}
	cmd.Flags().StringVar(&query, "query", "", "case-insensitive substring match on user name or email")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		self := false
		if len(args) == 1 {
			if !strings.EqualFold(strings.TrimSpace(args[0]), "self") {
				return &usageError{msg: fmt.Sprintf(`user get accepts only the positional value "self", got %q`, args[0])}
			}
			self = true
		}
		queried := cmd.Flags().Changed("query")
		if self && queried {
			return &usageError{msg: "user get: `self` and --query are mutually exclusive"}
		}
		if self {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		}
		if queried {
			return s.searchUsers(cmd.Context(), token, query)
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/users", nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// searchUsers lists every user (following pagination) and keeps those whose
// name or email contains query, case-insensitively. It emits a list envelope so
// the shape matches an unfiltered list; zero matches yields an empty results
// array rather than an error.
func (s *Service) searchUsers(ctx context.Context, token, query string) error {
	fetch := func(ctx context.Context, cursor string) ([]byte, error) {
		path := "/users"
		if cursor != "" {
			path += "?start_cursor=" + url.QueryEscape(cursor)
		}
		return s.call(ctx, token, http.MethodGet, path, nil)
	}
	body, err := paginate(ctx, true, "", fetch)
	if err != nil {
		return err
	}
	var env struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return &apiError{msg: fmt.Sprintf("notion: decode users list: %v", err), err: err}
	}
	needle := strings.ToLower(query)
	matched := []json.RawMessage{}
	for _, r := range env.Results {
		var u struct {
			Name   string `json:"name"`
			Person struct {
				Email string `json:"email"`
			} `json:"person"`
		}
		_ = json.Unmarshal(r, &u)
		if strings.Contains(strings.ToLower(u.Name), needle) ||
			strings.Contains(strings.ToLower(u.Person.Email), needle) {
			matched = append(matched, r)
		}
	}
	out, err := json.Marshal(map[string]any{
		"object": "list", "results": matched, "has_more": false, "next_cursor": nil,
	})
	if err != nil {
		return &apiError{msg: fmt.Sprintf("notion: encode users list: %v", err), err: err}
	}
	return s.emitJSON(out)
}

// newTeamListCmd is `team list` (GET /v1/teams): teamspaces under the 2026-03-11
// data model. Output JSON. A non-2xx surfaces as an apiError — the command never
// silently returns an empty list.
func (s *Service) newTeamListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teamspaces",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodGet, "/teams", nil, markdownVersion)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newTaskGetCmd is `task get <id>`: a single async-task status read. This is the
// manual fallback that is always available — the --allow-async write path polls
// this endpoint internally, and a caller can re-run `task get` to keep checking
// after an auto-poll timeout. Output JSON.
func (s *Service) newTaskGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Get an async task's status",
		Args:  cobra.ExactArgs(1),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		body, err := s.taskGet(cmd.Context(), token, id)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
