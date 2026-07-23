package missive

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newPostsCmd builds the `posts` group. A post is an internal comment /
// annotation injected into a conversation — Missive's headline write verb.
func (s *Service) newPostsCmd(token string) *cobra.Command {
	group := newGroupCmd("posts", "Inject internal posts (comments/annotations) into conversations")
	group.AddCommand(s.newPostsCreateCmd(token))
	return group
}

func (s *Service) newPostsCreateCmd(token string) *cobra.Command {
	var inline, file string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a post (POST /posts). Body: {\"posts\":{...}}",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := s.decodeJSONBody(inline, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/posts", nil, payload)
			if err != nil {
				return err
			}
			return s.emitBodyOrOK(body)
		},
	}
	addBodyFlags(cmd, &inline, &file)
	return cmd
}
