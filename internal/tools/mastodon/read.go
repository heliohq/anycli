package mastodon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// timelinePage is the provider-neutral list shape emitted by timelines,
// account posts, and search statuses. cursor is the max_id to pass back as
// --cursor for the next (older) page; empty when there is no next page.
type timelinePage struct {
	Posts  []statusSummary `json:"posts"`
	Cursor string          `json:"cursor,omitempty"`
}

func (rt *runContext) newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Short:       "Show the connected account (verify_credentials)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, _, err := rt.call(cmd.Context(), http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil)
			if err != nil {
				return err
			}
			account, err := decodeAccount(body)
			if err != nil {
				return err
			}
			return rt.emitJSON(detailFromAccount(account))
		},
	}
}

// registerPaging adds the shared --limit / --cursor flags to a list command.
func registerPaging(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 20, "maximum number of items to return")
	cmd.Flags().String("cursor", "", "pagination cursor (max_id) from a previous page")
}

// pagingQuery builds the limit/max_id query from the shared paging flags.
func pagingQuery(cmd *cobra.Command) url.Values {
	q := url.Values{}
	if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor, _ := cmd.Flags().GetString("cursor"); cursor != "" {
		q.Set("max_id", cursor)
	}
	return q
}

// listStatuses GETs a status-list endpoint and emits the provider-neutral
// timelinePage with the Link-header next cursor.
func (rt *runContext) listStatuses(ctx context.Context, path string, query url.Values) error {
	body, header, err := rt.call(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	list, err := decodeStatuses(body)
	if err != nil {
		return err
	}
	page := timelinePage{Posts: make([]statusSummary, 0, len(list)), Cursor: parseLinkCursor(header)}
	for _, s := range list {
		page.Posts = append(page.Posts, summarizeStatus(s))
	}
	return rt.emitJSON(page)
}

func (rt *runContext) newTimelineHomeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "home",
		Short:       "Read the home timeline",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rt.listStatuses(cmd.Context(), "/api/v1/timelines/home", pagingQuery(cmd))
		},
	}
	registerPaging(cmd)
	return cmd
}

func (rt *runContext) newTimelinePublicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "public",
		Short:       "Read the public (federated) or local timeline",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := pagingQuery(cmd)
			if local, _ := cmd.Flags().GetBool("local"); local {
				q.Set("local", "true")
			}
			return rt.listStatuses(cmd.Context(), "/api/v1/timelines/public", q)
		},
	}
	registerPaging(cmd)
	cmd.Flags().Bool("local", false, "restrict to this instance's local timeline")
	return cmd
}

func (rt *runContext) newTimelineTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "tag <hashtag>",
		Short:       "Read a hashtag timeline",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			tag := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
			path := "/api/v1/timelines/tag/" + url.PathEscape(tag)
			return rt.listStatuses(cmd.Context(), path, pagingQuery(cmd))
		},
	}
	registerPaging(cmd)
	return cmd
}

func (rt *runContext) newAccountGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "get <acct|id>",
		Short:       "Look up an account by @handle or numeric id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := rt.resolveAccountID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body, _, err := rt.call(cmd.Context(), http.MethodGet, "/api/v1/accounts/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			account, err := decodeAccount(body)
			if err != nil {
				return err
			}
			return rt.emitJSON(detailFromAccount(account))
		},
	}
}

func (rt *runContext) newAccountPostsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "posts <acct|id>",
		Short:       "Read an account's recent posts",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := rt.resolveAccountID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			path := "/api/v1/accounts/" + url.PathEscape(id) + "/statuses"
			return rt.listStatuses(cmd.Context(), path, pagingQuery(cmd))
		},
	}
	registerPaging(cmd)
	return cmd
}

func (rt *runContext) newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search accounts, hashtags, or statuses",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, _ := cmd.Flags().GetString("q")
			if strings.TrimSpace(query) == "" {
				return &usageError{msg: "search requires --q"}
			}
			q := url.Values{}
			q.Set("q", query)
			if typ, _ := cmd.Flags().GetString("type"); typ != "" {
				if typ != "accounts" && typ != "hashtags" && typ != "statuses" {
					return &usageError{msg: "invalid --type (want accounts, hashtags, or statuses)"}
				}
				q.Set("type", typ)
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				q.Set("limit", fmt.Sprintf("%d", limit))
			}
			body, _, err := rt.call(cmd.Context(), http.MethodGet, "/api/v2/search", q, nil)
			if err != nil {
				return err
			}
			return rt.emitSearch(body)
		},
	}
	cmd.Flags().String("q", "", "search query (required)")
	cmd.Flags().String("type", "", "restrict to accounts, hashtags, or statuses")
	cmd.Flags().Int("limit", 20, "maximum number of results per type")
	return cmd
}

// emitSearch reshapes the v2 search envelope. Accounts and statuses are
// summarized to the provider-neutral shapes; hashtags are surfaced as their
// names (the raw tag objects add no agent value).
func (rt *runContext) emitSearch(body []byte) error {
	var raw struct {
		Accounts []rawAccount `json:"accounts"`
		Statuses []rawStatus  `json:"statuses"`
		Hashtags []struct {
			Name string `json:"name"`
		} `json:"hashtags"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &apiError{msg: "mastodon: decode search response", err: err}
	}
	accounts := make([]accountDetail, 0, len(raw.Accounts))
	for _, a := range raw.Accounts {
		accounts = append(accounts, detailFromAccount(a))
	}
	statuses := make([]statusSummary, 0, len(raw.Statuses))
	for _, s := range raw.Statuses {
		statuses = append(statuses, summarizeStatus(s))
	}
	hashtags := make([]string, 0, len(raw.Hashtags))
	for _, h := range raw.Hashtags {
		hashtags = append(hashtags, h.Name)
	}
	return rt.emitJSON(map[string]any{
		"accounts": accounts,
		"statuses": statuses,
		"hashtags": hashtags,
	})
}

func (rt *runContext) newNotificationsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Read notifications (mentions, follows, boosts, favourites)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, header, err := rt.call(cmd.Context(), http.MethodGet, "/api/v1/notifications", pagingQuery(cmd), nil)
			if err != nil {
				return err
			}
			var raw []struct {
				ID        string     `json:"id"`
				Type      string     `json:"type"`
				CreatedAt string     `json:"created_at"`
				Account   rawAccount `json:"account"`
				Status    *rawStatus `json:"status"`
			}
			if err := json.Unmarshal(body, &raw); err != nil {
				return &apiError{msg: "mastodon: decode notifications response", err: err}
			}
			items := make([]map[string]any, 0, len(raw))
			for _, n := range raw {
				item := map[string]any{
					"id":         n.ID,
					"type":       n.Type,
					"created_at": n.CreatedAt,
					"account": accountRef{
						ID:          n.Account.ID,
						Acct:        n.Account.Acct,
						DisplayName: n.Account.DisplayName,
					},
				}
				if n.Status != nil {
					s := summarizeStatus(*n.Status)
					item["status"] = s
				}
				items = append(items, item)
			}
			return rt.emitJSON(map[string]any{"notifications": items, "cursor": parseLinkCursor(header)})
		},
	}
	registerPaging(cmd)
	return cmd
}

// resolveAccountID returns a numeric account id for a handle-or-id argument.
// An all-digit argument is already an id; anything else (a @handle or a bare
// username) is resolved via GET /api/v1/accounts/lookup?acct=… so the AI and
// humans speak handles, not per-instance numeric ids.
func (rt *runContext) resolveAccountID(ctx context.Context, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", &usageError{msg: "account argument is required"}
	}
	if isNumericID(arg) {
		return arg, nil
	}
	acct := strings.TrimPrefix(arg, "@")
	q := url.Values{}
	q.Set("acct", acct)
	body, _, err := rt.call(ctx, http.MethodGet, "/api/v1/accounts/lookup", q, nil)
	if err != nil {
		return "", err
	}
	account, err := decodeAccount(body)
	if err != nil {
		return "", err
	}
	if account.ID == "" {
		return "", &apiError{msg: fmt.Sprintf("mastodon: account %q not found", arg)}
	}
	return account.ID, nil
}

func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
