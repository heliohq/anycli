package x

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type createPostRequest struct {
	Text        string     `json:"text"`
	Media       *postMedia `json:"media,omitempty"`
	Reply       *postReply `json:"reply,omitempty"`
	QuotePostID string     `json:"quote_tweet_id,omitempty"`
}

type postMedia struct {
	MediaIDs []string `json:"media_ids"`
}

type postReply struct {
	InReplyToPostID string `json:"in_reply_to_tweet_id"`
}

func (s *Service) newPostCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "post", Short: "Posts"}
	cmd.AddCommand(
		s.newPostGetCmd(token),
		s.newPostSearchCmd(token),
		s.newPostCreateCmd(token),
		s.newPostReplyCmd(token),
		s.newPostRepliesCmd(token),
		s.newPostQuoteCmd(token),
		s.newPostQuotesCmd(token),
		s.newPostHiddenCmd(token, "hide", "Hide a reply (comment) under one of your posts", longPostHide, true),
		s.newPostHiddenCmd(token, "unhide", "Unhide a previously hidden reply", longPostUnhide, false),
		s.newPostThreadCmd(token),
		s.newPostDeleteCmd(token),
		s.newPostAudienceCmd(token, "liking-users", "Users who liked a post", longPostLikingUsers, "liking_users"),
		s.newPostAudienceCmd(token, "reposters", "Users who reposted a post", longPostReposters, "retweeted_by"),
	)
	return cmd
}

func (s *Service) newPostGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <post-id>...",
		Short: "Get one or more posts (up to 100) by id",
		Long: "Not bound by the 7-day recent-search window, so this is the only way to\n" +
			"read a post older than a week. Batch the ids instead of looping: one call\n" +
			"with many ids costs the same as one call with one. The response shape\n" +
			"follows the id count — a single id returns `data` as an object, two or more\n" +
			"return `data` as an array.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 100 {
				return fmt.Errorf("at most 100 post ids are supported per lookup")
			}
			for _, id := range args {
				if err := requireNumericID("post id", id); err != nil {
					return err
				}
			}
			query := url.Values{"tweet.fields": {defaultPostFields}}
			path := "/2/tweets/" + url.PathEscape(args[0])
			if len(args) > 1 {
				path = "/2/tweets"
				query.Set("ids", strings.Join(args, ","))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newPostSearchCmd(token string) *cobra.Command {
	var query, nextToken, sinceID, sortOrder string
	var limit int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search recent posts (one page)",
		Long: "Rides X's recent-search endpoint: the last 7 days only, with no\n" +
			"full-archive access on this API tier — `post get` still reads older posts\n" +
			"by id. --query takes X search operators (from:, to:, conversation_id:,\n" +
			"-is:retweet, ...). --limit is 10-100, default 10; the API rejects smaller\n" +
			"pages. --sort-order is recency (default) or relevancy. For engagement on\n" +
			"the connected account's OWN posts prefer `timeline mentions --since-id`,\n" +
			"which is cheaper and not capped at 7 days.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("query is required")
			}
			if err := requireLimit(limit, 10, 100); err != nil {
				return err
			}
			if err := requireOptionalNumericID("since id", sinceID); err != nil {
				return err
			}
			if err := requireSortOrder(sortOrder); err != nil {
				return err
			}
			values := url.Values{
				"query":        {query},
				"max_results":  {strconv.Itoa(limit)},
				"tweet.fields": {defaultPostFields},
			}
			if nextToken != "" {
				values.Set("next_token", nextToken)
			}
			if sinceID != "" {
				values.Set("since_id", sinceID)
			}
			if sortOrder != "" {
				values.Set("sort_order", sortOrder)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/tweets/search/recent", values, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "recent-post search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (10-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	cmd.Flags().StringVar(&sinceID, "since-id", "", "only posts newer than this post id")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", `result order: "recency" or "relevancy"`)
	return cmd
}

func (s *Service) newPostCreateCmd(token string) *cobra.Command {
	var text, replyTo string
	var mediaIDs []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a post",
		Long: "--media-id attaches media already uploaded with `media upload` and is\n" +
			"repeatable up to 4 times; X accepts either up to 4 images, or exactly one\n" +
			"video or GIF, never a mix. --reply-to <post-id> turns this into a reply —\n" +
			"the same result as `post reply`, except this command can also attach media\n" +
			"to that reply. Post length is enforced by X and not checked locally (280\n" +
			"characters on a standard account, more on paid tiers).",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := buildCreatePostRequest(text, mediaIDs, replyTo, "")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/tweets", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "post text")
	cmd.Flags().StringArrayVar(&mediaIDs, "media-id", nil, "uploaded media id (repeatable, maximum 4)")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "post id to reply to")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func (s *Service) newPostReplyCmd(token string) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "reply <post-id>",
		Short: "Reply to a post",
		Long: "The reply is a full post: it gets its own id, its own engagement and its\n" +
			"own reply tree, and it joins the target's conversation. There is no\n" +
			"--media-id here — to reply with an image use `post create --reply-to\n" +
			"<post-id> --media-id <id>`.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := buildCreatePostRequest(text, nil, args[0], "")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/tweets", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "reply text")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func (s *Service) newPostThreadCmd(token string) *cobra.Command {
	var texts []string
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Create a thread from repeated --text values",
		Long: "Posts each value in order, each one replying to the previous, and prints\n" +
			"one JSON object per created post. At least two values are required. It is\n" +
			"NOT atomic: a failure on post 3 leaves posts 1 and 2 published — the error\n" +
			"names the failing index and cleanup is a manual `post delete` per id. No\n" +
			"--media-id here; a thread with media has to be built by hand from `post\n" +
			"create --media-id` plus `post create --reply-to <previous-id> --media-id`.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(texts) < 2 {
				return fmt.Errorf("at least two --text values are required")
			}
			previousID := ""
			for index, text := range texts {
				payload, err := buildCreatePostRequest(text, nil, previousID, "")
				if err != nil {
					return fmt.Errorf("thread post %d: %w", index+1, err)
				}
				body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/tweets", nil, payload)
				if err != nil {
					return fmt.Errorf("thread post %d: %w", index+1, err)
				}
				previousID, err = createdPostID(body)
				if err != nil {
					return fmt.Errorf("thread post %d: %w", index+1, err)
				}
				if err := s.emit(body); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&texts, "text", nil, "post text in thread order (repeatable)")
	return cmd
}

func (s *Service) newPostDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <post-id>",
		Short: "Delete a post",
		Long: "Only the connected account's own posts can be deleted, and deletion is\n" +
			"irreversible — the id is not reusable afterwards. Deleting the root of a\n" +
			"conversation does not delete the replies; they stay in the conversation\n" +
			"with the removed post shown as unavailable.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("post id", args[0]); err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodDelete, "/2/tweets/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func buildCreatePostRequest(text string, mediaIDs []string, replyTo, quotePostID string) (createPostRequest, error) {
	if strings.TrimSpace(text) == "" {
		return createPostRequest{}, fmt.Errorf("text must not be empty")
	}
	if len(mediaIDs) > 4 {
		return createPostRequest{}, fmt.Errorf("at most four --media-id values are supported")
	}
	for _, mediaID := range mediaIDs {
		if err := requireNumericID("media id", mediaID); err != nil {
			return createPostRequest{}, err
		}
	}
	if replyTo != "" {
		if err := requireNumericID("reply-to post id", replyTo); err != nil {
			return createPostRequest{}, err
		}
	}
	if quotePostID != "" {
		if err := requireNumericID("quoted post id", quotePostID); err != nil {
			return createPostRequest{}, err
		}
	}

	payload := createPostRequest{Text: text, QuotePostID: quotePostID}
	if len(mediaIDs) > 0 {
		payload.Media = &postMedia{MediaIDs: mediaIDs}
	}
	if replyTo != "" {
		payload.Reply = &postReply{InReplyToPostID: replyTo}
	}
	return payload, nil
}

func createdPostID(body []byte) (string, error) {
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode create-post response: %w", err)
	}
	if response.Data.ID == "" {
		return "", fmt.Errorf("create-post response has no data.id")
	}
	return response.Data.ID, nil
}
