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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
