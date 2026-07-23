package instagram

import (
	"net/url"

	"github.com/spf13/cobra"
)

// commentFields are the default fields for `comment list`, including nested
// replies.
const commentFields = "id,text,username,timestamp,like_count,replies{id,text,username,timestamp,like_count}"

func (s *Service) newCommentListCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "list <media_id>",
		Short: "List a media object's comments (GET /{media_id}/comments)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("fields", firstNonEmpty(fields, commentFields))
			body, err := s.get(cmd.Context(), token, "/"+url.PathEscape(args[0])+"/comments", q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field list (default: comment summary + replies)")
	return cmd
}

func (s *Service) newCommentReplyCmd(token string) *cobra.Command {
	var message string
	cmd := &cobra.Command{
		Use:   "reply <comment_id>",
		Short: "Reply to a comment (POST /{comment_id}/replies)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if message == "" {
				return &usageError{msg: "--message is required"}
			}
			form := url.Values{}
			form.Set("message", message)
			body, err := s.postForm(cmd.Context(), token, "/"+url.PathEscape(args[0])+"/replies", form)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "reply text (required)")
	return cmd
}

func (s *Service) newCommentHideCmd(token string) *cobra.Command {
	// A string flag (not bool) so both `--hidden true` and `--hidden false`
	// parse as a value — a bare cobra bool flag would treat `--hidden false`
	// as a positional argument.
	var hidden string
	cmd := &cobra.Command{
		Use:   "hide <comment_id>",
		Short: "Hide or unhide a comment (POST /{comment_id} hide=true|false)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if hidden != "true" && hidden != "false" {
				return &usageError{msg: "--hidden must be true or false"}
			}
			form := url.Values{}
			form.Set("hide", hidden)
			body, err := s.postForm(cmd.Context(), token, "/"+url.PathEscape(args[0]), form)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&hidden, "hidden", "true", "true to hide, false to unhide")
	return cmd
}

func (s *Service) newCommentDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <comment_id>",
		Short: "Delete a comment (DELETE /{comment_id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.del(cmd.Context(), token, "/"+url.PathEscape(args[0]))
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	return cmd
}
