package reddit

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newCommentCmd groups comment writes: create (reply), edit, delete.
func (s *Service) newCommentCmd(token string) *cobra.Command {
	cmd := newGroup("comment", "Reply to and manage your comments")
	cmd.AddCommand(
		s.newCommentCreateCmd(token),
		s.newCommentEditCmd(token),
		s.newCommentDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newCommentCreateCmd(token string) *cobra.Command {
	var parent, text string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Reply to a post or comment (--parent is its fullname)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFullname(parent); err != nil {
				return err
			}
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			form := url.Values{"api_type": {"json"}, "thing_id": {parent}, "text": {text}}
			body, err := s.postForm(cmd.Context(), token, "/api/comment", form)
			if err != nil {
				return err
			}
			env, err := checkJSONErrors(body)
			if err != nil {
				return err
			}
			if d, ok := createdThing(env); ok {
				return s.emitCreated(cmd, d.ID, d.Name, d.Permalink)
			}
			return s.emitCreated(cmd, "", "", "")
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "fullname of the post or comment to reply to (required)")
	cmd.Flags().StringVar(&text, "text", "", "comment body markdown (required)")
	return cmd
}

func (s *Service) newCommentEditCmd(token string) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:         "edit <fullname>",
		Short:       "Edit your own comment (t1_ fullname)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFullname(args[0]); err != nil {
				return err
			}
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			return s.editUserText(cmd, token, args[0], text)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "new comment body markdown (required)")
	return cmd
}

func (s *Service) newCommentDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <fullname>",
		Short:       "Delete your own comment (t1_ fullname)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.deleteThing(cmd, token, args[0])
		},
	}
}
