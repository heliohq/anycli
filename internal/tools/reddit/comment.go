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
		Use:   "create",
		Short: "Reply to a post or comment (--parent is its fullname)",
		Long: "The reply command for both levels: `--parent` takes the FULLNAME of what\n" +
			"is being replied to — `t3_…` for a top-level reply to a post, `t1_…` to\n" +
			"answer another comment — and a bare id is rejected before anything is\n" +
			"sent. `--text` is Reddit markdown. The comment is public immediately\n" +
			"under the connected account, and the response carries its own\n" +
			"`fullname` and `permalink`, which are what a later edit or delete\n" +
			"needs.",
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
		Use:   "edit <fullname>",
		Short: "Edit your own comment (t1_ fullname)",
		Long: "Takes the comment's `t1_` fullname and replaces the body with `--text` in\n" +
			"full; there is no partial edit. Only the connected account's own\n" +
			"comments can be edited, and Reddit marks the result as edited for\n" +
			"everyone who reads it.",
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
		Use:   "delete <fullname>",
		Short: "Delete your own comment (t1_ fullname)",
		Long: "Takes the `t1_` fullname; immediate and not reversible. Replies beneath\n" +
			"the comment survive, re-parented under a deletion placeholder, so\n" +
			"removing a comment does not remove the conversation hanging off it.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.deleteThing(cmd, token, args[0])
		},
	}
}
