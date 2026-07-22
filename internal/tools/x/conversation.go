package x

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newPostRepliesCmd lists the replies (comments) in a post's conversation via
// recent search with the conversation_id operator — the official X API path
// for reading replies; recent search only covers the last 7 days. The
// conversation id is resolved first because a reply's conversation_id is the
// root post's id, not its own.
func (s *Service) newPostRepliesCmd(token string) *cobra.Command {
	var nextToken, sinceID string
	var limit int
	cmd := &cobra.Command{
		Use:         "replies <post-id>",
		Annotations: sideEffect(false),
		Short:       "List replies (comments) in a post's conversation (one page, last 7 days)",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("post id", args[0]); err != nil {
				return err
			}
			if err := requireLimit(limit, 10, 100); err != nil {
				return err
			}
			if err := requireOptionalNumericID("since id", sinceID); err != nil {
				return err
			}
			conversationID, err := s.resolveConversationID(cmd.Context(), token, args[0])
			if err != nil {
				return err
			}
			values := url.Values{
				"query":        {"conversation_id:" + conversationID},
				"max_results":  {strconv.Itoa(limit)},
				"tweet.fields": {defaultPostFields},
			}
			if nextToken != "" {
				values.Set("next_token", nextToken)
			}
			if sinceID != "" {
				values.Set("since_id", sinceID)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/tweets/search/recent", values, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (10-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	cmd.Flags().StringVar(&sinceID, "since-id", "", "only replies newer than this post id")
	return cmd
}

func (s *Service) newPostQuotesCmd(token string) *cobra.Command {
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:         "quotes <post-id>",
		Annotations: sideEffect(false),
		Short:       "List quote posts of a post (one page)",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("post id", args[0]); err != nil {
				return err
			}
			if err := requireLimit(limit, 10, 100); err != nil {
				return err
			}
			values := url.Values{
				"max_results":  {strconv.Itoa(limit)},
				"tweet.fields": {defaultPostFields},
			}
			if nextToken != "" {
				values.Set("pagination_token", nextToken)
			}
			path := "/2/tweets/" + url.PathEscape(args[0]) + "/quote_tweets"
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, values, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (10-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}

func (s *Service) newPostQuoteCmd(token string) *cobra.Command {
	var text string
	var mediaIDs []string
	cmd := &cobra.Command{
		Use:         "quote <post-id>",
		Annotations: sideEffect(true),
		Short:       "Quote a post with a comment",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := buildCreatePostRequest(text, mediaIDs, "", args[0])
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
	cmd.Flags().StringVar(&text, "text", "", "quote post text")
	cmd.Flags().StringArrayVar(&mediaIDs, "media-id", nil, "uploaded media id (repeatable, maximum 4)")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// newPostHiddenCmd builds `post hide` / `post unhide`: moderation of replies
// under the connected user's own posts (PUT /2/tweets/:id/hidden).
func (s *Service) newPostHiddenCmd(token, use, short string, hidden bool) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <reply-id>",
		Annotations: sideEffect(true),
		Short:       short,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("reply id", args[0]); err != nil {
				return err
			}
			path := "/2/tweets/" + url.PathEscape(args[0]) + "/hidden"
			body, err := s.call(cmd.Context(), token, http.MethodPut, path, nil, struct {
				Hidden bool `json:"hidden"`
			}{Hidden: hidden})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) resolveConversationID(ctx context.Context, token, postID string) (string, error) {
	query := url.Values{"tweet.fields": {"conversation_id"}}
	body, err := s.call(ctx, token, http.MethodGet, "/2/tweets/"+url.PathEscape(postID), query, nil)
	if err != nil {
		return "", fmt.Errorf("resolve conversation for post %s: %w", postID, err)
	}
	var response struct {
		Data struct {
			ConversationID string `json:"conversation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode conversation lookup for post %s: %w", postID, err)
	}
	if response.Data.ConversationID == "" {
		return "", fmt.Errorf("post %s lookup returned no conversation_id", postID)
	}
	return response.Data.ConversationID, nil
}
