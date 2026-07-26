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
		Long: "Runs as a recent search over the post's conversation id, so it returns\n" +
			"EVERY post in the conversation, not only direct children of the id you\n" +
			"passed — rebuild nesting from each item's `referenced_tweets` entry of\n" +
			"type `replied_to`. Passing a reply's id works: it resolves to the same\n" +
			"conversation as the root post. Costs two API calls, one lookup to resolve\n" +
			"the conversation and one search. --limit is 10-100, default 10; continue\n" +
			"with --next-token, or poll incrementally with --since-id.",
		Args: cobra.ExactArgs(1),
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
		Long: "Reads the quote_tweets endpoint rather than recent search, so unlike\n" +
			"`post replies` it is not limited to the last 7 days. --limit is 10-100,\n" +
			"default 10; continue with --next-token. There is no --since-id, so\n" +
			"watching for new quotes means re-reading from the first page.",
		Args: cobra.ExactArgs(1),
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
		Long: "Creates a NEW post that embeds the target, so it carries its own id,\n" +
			"engagement and reply tree — unlike `repost create`, which adds no text and\n" +
			"produces nothing anyone can reply to you on. --media-id attaches up to 4\n" +
			"already-uploaded media. Deleting the quote leaves the quoted post\n" +
			"untouched.",
		Args: cobra.ExactArgs(1),
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

// longPostHide and longPostUnhide are the two moderation Longs. They live next
// to the shared builder because it is the builder that fixes the argument
// (a reply id) and the endpoint both describe.
const (
	longPostHide = "Takes the REPLY's id, not the id of the post it sits under. Only replies\n" +
		"under the connected account's own posts can be hidden; anything else\n" +
		"returns 403. Hiding neither deletes the reply nor notifies its author — it\n" +
		"moves the reply behind X's \"Show additional replies\" affordance. Reverse\n" +
		"with `post unhide`."

	longPostUnhide = "Takes the REPLY's id, not the id of the post it sits under, and carries\n" +
		"the same ownership rule as `post hide`: only replies under the connected\n" +
		"account's own posts, 403 otherwise. The response reports the resulting\n" +
		"state, so calling it on a reply that was never hidden is not an error."
)

// newPostHiddenCmd builds `post hide` / `post unhide`: moderation of replies
// under the connected user's own posts (PUT /2/tweets/:id/hidden).
func (s *Service) newPostHiddenCmd(token, use, short, long string, hidden bool) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <reply-id>",
		Annotations: sideEffect(true),
		Short:       short,
		Long:        long,
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
