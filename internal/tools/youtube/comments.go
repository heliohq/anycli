package youtube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// validCommentOrders are the accepted --order values for comment threads.
var validCommentOrders = map[string]bool{"time": true, "relevance": true}

// validModerationStatuses are the accepted --status values for moderation.
var validModerationStatuses = map[string]bool{
	"heldForReview": true, "published": true, "rejected": true,
}

// newCommentsListCmd lists top-level comment threads on a video, with each
// thread's replies hydrated (part=snippet,replies).
func (s *Service) newCommentsListCmd(token string) *cobra.Command {
	var video, order string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List top-level comment threads on a video",
		Long: "Returns TOP-LEVEL threads with each thread's replies hydrated inline, so\n" +
			"the common case needs no follow-up call — reach for `comments replies`\n" +
			"only when a thread has more replies than the API embedded. --order is time\n" +
			"or relevance; time with a recorded high-water mark is how to poll for new\n" +
			"comments. Note the ids here are thread ids, while `comments reply`,\n" +
			"`comments update`, `comments delete` and `comments moderate` all take a\n" +
			"COMMENT id. --max is capped at 50 and defaults to 5.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if video == "" {
				return &usageError{msg: "--video is required"}
			}
			if order != "" && !validCommentOrders[order] {
				return &usageError{msg: fmt.Sprintf("--order must be time or relevance, got %q", order)}
			}
			q := url.Values{}
			q.Set("part", "snippet,replies")
			q.Set("videoId", video)
			if order != "" {
				q.Set("order", order)
			}
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/commentThreads", q, nil)
			if err != nil {
				return err
			}
			lr, err := decodeList(body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitList(lr)
			}
			return s.renderCommentThreads(lr)
		},
	}
	cmd.Flags().StringVar(&video, "video", "", "video id")
	cmd.Flags().StringVar(&order, "order", "", "sort order: time|relevance")
	addListFlags(cmd, &max, &page)
	return cmd
}

// newCommentsRepliesCmd lists the replies under one top-level comment.
func (s *Service) newCommentsRepliesCmd(token string) *cobra.Command {
	var parent string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:   "replies",
		Short: "List replies under a top-level comment",
		Long: "--parent is the top-level comment id, not a video id and not a thread id.\n" +
			"Only needed when a thread carries more replies than `comments list`\n" +
			"embedded — otherwise it is a redundant request. Replies are one level deep\n" +
			"on YouTube: a reply to a reply is still attached to the same top-level\n" +
			"comment.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if parent == "" {
				return &usageError{msg: "--parent is required (the top-level comment id)"}
			}
			q := url.Values{}
			q.Set("part", "snippet")
			q.Set("parentId", parent)
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/comments", q, nil)
			if err != nil {
				return err
			}
			lr, err := decodeList(body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitList(lr)
			}
			return s.renderComments(lr)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "top-level comment id")
	addListFlags(cmd, &max, &page)
	return cmd
}

// newCommentsReplyCmd posts a reply under a top-level comment.
func (s *Service) newCommentsReplyCmd(token string) *cobra.Command {
	var parent, text string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to a top-level comment",
		Long: "Posts publicly as the connected channel, visible immediately under the\n" +
			"video. --parent must be a TOP-LEVEL comment id: YouTube has no second\n" +
			"nesting level, so replying to a reply means passing that reply's parent.\n" +
			"There is no way to start a new top-level comment from this tool — only\n" +
			"replies.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if parent == "" || text == "" {
				return &usageError{msg: "--parent and --text are required"}
			}
			payload := map[string]any{"snippet": map[string]any{"parentId": parent, "textOriginal": text}}
			q := url.Values{}
			q.Set("part", "snippet")
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/comments", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderPostedComment(body, "replied")
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "top-level comment id")
	cmd.Flags().StringVar(&text, "text", "", "reply body")
	return cmd
}

// newCommentsUpdateCmd edits the text of the connected user's own comment.
func (s *Service) newCommentsUpdateCmd(token string) *cobra.Command {
	var id, text string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Edit the text of your own comment",
		Long: "Only reaches comments the connected account authored; someone else's\n" +
			"comment fails rather than being edited, and the lever for those is\n" +
			"`comments moderate`. The whole text is replaced, and YouTube marks the\n" +
			"comment as edited publicly.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" || text == "" {
				return &usageError{msg: "--id and --text are required"}
			}
			payload := map[string]any{"id": id, "snippet": map[string]any{"textOriginal": text}}
			q := url.Values{}
			q.Set("part", "snippet")
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/comments", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderPostedComment(body, "updated")
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "comment id")
	cmd.Flags().StringVar(&text, "text", "", "new comment body")
	return cmd
}

func (s *Service) newCommentsDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a comment",
		Long: "Permanent, with no undo. It removes the comment outright, which is the\n" +
			"harsher of the two options — `comments moderate --status rejected` hides a\n" +
			"comment from viewers while leaving it recoverable, and is usually what\n" +
			"\"remove this comment\" should mean for someone else's post.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			q := url.Values{}
			q.Set("id", id)
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/comments", q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitOK(id)
			}
			fmt.Fprintf(s.stdout(), "deleted comment %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "comment id")
	return cmd
}

// newCommentsModerateCmd sets a comment's moderation status. --ban-author is
// valid ONLY with --status rejected (the API returns 400 banWithoutReject
// otherwise), so the combination is rejected client-side before the call.
func (s *Service) newCommentsModerateCmd(token string) *cobra.Command {
	var id, status string
	var banAuthor bool
	cmd := &cobra.Command{
		Use:   "moderate",
		Short: "Set a comment's moderation status (heldForReview | published | rejected)",
		Long: "The lever for other people's comments on the channel's videos: rejected\n" +
			"hides it from viewers, heldForReview parks it in the moderation queue, and\n" +
			"published releases one that was held. Unlike `comments delete` none of\n" +
			"this destroys the comment. --ban-author is valid ONLY with --status\n" +
			"rejected and is refused locally otherwise, because the API answers 400\n" +
			"banWithoutReject; banning hides the author's future comments on the\n" +
			"channel too, which is a standing decision rather than a one-off.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			if !validModerationStatuses[status] {
				return &usageError{msg: fmt.Sprintf("--status must be heldForReview|published|rejected, got %q", status)}
			}
			if banAuthor && status != "rejected" {
				return &usageError{msg: "--ban-author is only valid with --status rejected"}
			}
			q := url.Values{}
			q.Set("id", id)
			q.Set("moderationStatus", status)
			if banAuthor {
				q.Set("banAuthor", "true")
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/comments/setModerationStatus", q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitOK(id)
			}
			fmt.Fprintf(s.stdout(), "set moderation status of %s to %s\n", id, status)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "comment id")
	cmd.Flags().StringVar(&status, "status", "", "heldForReview|published|rejected")
	cmd.Flags().BoolVar(&banAuthor, "ban-author", false, "also ban the author (only valid with --status rejected)")
	return cmd
}

// renderCommentThreads prints the top-level comment of each thread plus its
// reply count.
func (s *Service) renderCommentThreads(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no comments")
		return nil
	}
	for _, raw := range lr.Items {
		var t struct {
			ID      string `json:"id"`
			Snippet struct {
				TopLevelComment struct {
					Snippet commentSnippet `json:"snippet"`
				} `json:"topLevelComment"`
				TotalReplyCount int64 `json:"totalReplyCount"`
			} `json:"snippet"`
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode comment thread: %v", err), err: err}
		}
		c := t.Snippet.TopLevelComment.Snippet
		fmt.Fprintf(s.stdout(), "%s\t%s: %s (%d replies)\n",
			t.ID, c.AuthorDisplayName, truncate(c.TextDisplay, 100), t.Snippet.TotalReplyCount)
	}
	if lr.NextPageToken != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", lr.NextPageToken)
	}
	return nil
}

func (s *Service) renderComments(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no replies")
		return nil
	}
	for _, raw := range lr.Items {
		var c struct {
			ID      string         `json:"id"`
			Snippet commentSnippet `json:"snippet"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode comment: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s\t%s: %s\n", c.ID, c.Snippet.AuthorDisplayName, truncate(c.Snippet.TextDisplay, 100))
	}
	if lr.NextPageToken != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", lr.NextPageToken)
	}
	return nil
}

func (s *Service) renderPostedComment(body []byte, verb string) error {
	var c struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &c)
	fmt.Fprintf(s.stdout(), "%s comment %s\n", verb, c.ID)
	return nil
}

// commentSnippet is the subset of a comment snippet used in human summaries.
type commentSnippet struct {
	AuthorDisplayName string `json:"authorDisplayName"`
	TextDisplay       string `json:"textDisplay"`
}
