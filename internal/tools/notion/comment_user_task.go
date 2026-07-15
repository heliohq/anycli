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

// newCommentCreateCmd is `comment create` (POST /v1/comments). Exactly one
// target: --page-id (comment on a page), --block-id (comment on a specific
// block), or --discussion-id (reply into an existing discussion thread) — the
// three are mutually exclusive. --content is Notion-flavored **markdown** (sent
// via the endpoint's `markdown` field), so inline bold/italic/strikethrough/
// code/links, inline equations, and @mentions all work. Output JSON.
func (s *Service) newCommentCreateCmd(token string) *cobra.Command {
	var pageID, blockID, discussionID, content string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Comment on a page/block, or reply to a discussion",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&pageID, "page-id", "", "page to comment on")
	cmd.Flags().StringVar(&blockID, "block-id", "", "block to comment on")
	cmd.Flags().StringVar(&discussionID, "discussion-id", "", "existing discussion thread to reply into")
	cmd.Flags().StringVar(&content, "content", "", "markdown comment body (required)")
	_ = cmd.MarkFlagRequired("content")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{"markdown": content}
		set := 0
		if cmd.Flags().Changed("page-id") {
			set++
			id, err := resolveID(pageID)
			if err != nil {
				return err
			}
			payload["parent"] = map[string]any{"page_id": id}
		}
		if cmd.Flags().Changed("block-id") {
			set++
			id, err := resolveID(blockID)
			if err != nil {
				return err
			}
			payload["parent"] = map[string]any{"block_id": id}
		}
		if cmd.Flags().Changed("discussion-id") {
			set++
			id, err := resolveID(discussionID)
			if err != nil {
				return err
			}
			payload["discussion_id"] = id
		}
		if set != 1 {
			return &usageError{msg: "comment create requires exactly one of --page-id, --block-id, --discussion-id"}
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

// newUserGetCmd is `user get [self|<user-id>] [--query <name|email>]` — four
// mutually exclusive forms: bare `self` → GET /v1/users/me; a `<user-id>` →
// GET /v1/users/{id} (resolve a specific user, incl. guests, that list omits);
// --query → list every user and keep case-insensitive substring matches on
// name OR email (0 hits is an empty list, not an error); no arg → GET /v1/users
// paginated through every page, aggregated into one list envelope (design 304
// §user "列出所有用户(分页)"). A positional and --query together is a usage
// error. Output JSON.
func (s *Service) newUserGetCmd(token string) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "get [self|<user-id>]",
		Short: "Get the current user (self), a user by id, search users (--query), or list all users",
		Args:  cobra.MaximumNArgs(1),
	}
	cmd.Flags().StringVar(&query, "query", "", "case-insensitive substring match on user name or email")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		self := false
		userID := ""
		if len(args) == 1 {
			if strings.EqualFold(strings.TrimSpace(args[0]), "self") {
				self = true
			} else {
				id, err := resolveID(args[0])
				if err != nil {
					return err
				}
				userID = id
			}
		}
		queried := cmd.Flags().Changed("query")
		if (self || userID != "") && queried {
			return &usageError{msg: "user get: a positional (self / user-id) and --query are mutually exclusive"}
		}
		if self {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		}
		if userID != "" {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/"+url.PathEscape(userID), nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		}
		if queried {
			return s.searchUsers(cmd.Context(), token, query)
		}
		// No arg: enumerate every user (paginated) into one list envelope so a
		// workspace with >100 members is not silently truncated to the first page.
		body, err := paginate(cmd.Context(), true, "", s.usersFetch(token))
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// usersFetch returns a page fetcher over GET /v1/users, threading start_cursor.
// Shared by `user get` (no arg) and searchUsers so both enumerate identically.
func (s *Service) usersFetch(token string) pageFetcher {
	return func(ctx context.Context, cursor string) ([]byte, error) {
		path := "/users"
		if cursor != "" {
			path += "?start_cursor=" + url.QueryEscape(cursor)
		}
		return s.call(ctx, token, http.MethodGet, path, nil)
	}
}

// searchUsers lists every user (following pagination) and keeps those whose
// name or email contains query, case-insensitively. It emits a list envelope so
// the shape matches an unfiltered list; zero matches yields an empty results
// array rather than an error.
func (s *Service) searchUsers(ctx context.Context, token, query string) error {
	body, err := paginate(ctx, true, "", s.usersFetch(token))
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
