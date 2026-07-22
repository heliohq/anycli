package figma

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

type commentPayload struct {
	Message    string         `json:"message"`
	CommentID  string         `json:"comment_id,omitempty"`
	ClientMeta map[string]any `json:"client_meta,omitempty"`
}

func (s *Service) newCommentsCommand(token string) *cobra.Command {
	comments := &cobra.Command{Use: "comments", Short: "Read and manage Figma file comments"}
	comments.AddCommand(
		s.newCommentsListCommand(token),
		s.newCommentPostCommand(token),
		s.newCommentDeleteCommand(token),
		s.newCommentReactionsCommand(token),
	)
	return comments
}

func (s *Service) newCommentReactionsCommand(token string) *cobra.Command {
	reactions := &cobra.Command{Use: "reactions", Short: "Read and manage comment reactions"}
	reactions.AddCommand(
		s.newOperationCommand(token, operationCommandSpec{Use: "list", Short: "List reactions on a comment", OperationID: "getCommentReactions"}),
		s.newCommentReactionAddCommand(token),
		s.newOperationCommand(token, operationCommandSpec{Use: "delete", Short: "Delete your reaction from a comment", OperationID: "deleteCommentReaction"}),
	)
	return reactions
}

func (s *Service) newCommentReactionAddCommand(token string) *cobra.Command {
	var fileKey, commentID, emoji string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a reaction to a comment",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "postCommentReaction",
			sideEffectAnnotation:  "true", // POST comment reaction
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := []string{"file_key=" + fileKey, "comment_id=" + commentID}
			payload := struct {
				Emoji string `json:"emoji"`
			}{Emoji: emoji}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "postCommentReaction", params, payload)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&commentID, "comment-id", "", "Figma comment ID")
	cmd.Flags().StringVar(&emoji, "emoji", "", "Figma reaction emoji, such as +1 or heart")
	_ = cmd.MarkFlagRequired("file-key")
	_ = cmd.MarkFlagRequired("comment-id")
	_ = cmd.MarkFlagRequired("emoji")
	return cmd
}

func (s *Service) newCommentsListCommand(token string) *cobra.Command {
	var fileKey string
	var asMarkdown bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List comments on a Figma file",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getComments",
			sideEffectAnnotation:  "false", // GET file comments
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if asMarkdown {
				query.Set("as_md", "true")
			}
			params := appendOperationQuery([]string{"file_key=" + fileKey}, query)
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getComments", params, nil)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().BoolVar(&asMarkdown, "as-md", false, "return comments as Markdown when possible")
	_ = cmd.MarkFlagRequired("file-key")
	return cmd
}

func (s *Service) newCommentPostCommand(token string) *cobra.Command {
	var fileKey, message, parentCommentID, clientMetaJSON string
	cmd := &cobra.Command{
		Use:   "post",
		Short: "Post a comment or reply on a Figma file",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "postComment",
			sideEffectAnnotation:  "true", // POST file comment
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientMeta, err := parseClientMeta(clientMetaJSON)
			if err != nil {
				return err
			}
			payload := commentPayload{
				Message:    message,
				CommentID:  parentCommentID,
				ClientMeta: clientMeta,
			}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "postComment", []string{"file_key=" + fileKey}, payload)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&message, "message", "", "comment text")
	cmd.Flags().StringVar(&parentCommentID, "comment-id", "", "root comment ID to reply to")
	cmd.Flags().StringVar(&clientMetaJSON, "client-meta-json", "", "JSON object describing the comment position")
	_ = cmd.MarkFlagRequired("file-key")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func (s *Service) newCommentDeleteCommand(token string) *cobra.Command {
	var fileKey, commentID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Figma file comment",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "deleteComment",
			sideEffectAnnotation:  "true", // DELETE file comment
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := []string{"file_key=" + fileKey, "comment_id=" + commentID}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "deleteComment", params, nil)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&commentID, "comment-id", "", "Figma comment ID")
	_ = cmd.MarkFlagRequired("file-key")
	_ = cmd.MarkFlagRequired("comment-id")
	return cmd
}

func parseClientMeta(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("--client-meta-json must be a JSON object: %w", err)
	}
	if value == nil {
		return nil, fmt.Errorf("--client-meta-json must be a JSON object")
	}
	return value, nil
}
