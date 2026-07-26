package typefully

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTagCmd groups tag list/create — tags are used to filter drafts.
func (s *Service) newTagCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "List and create draft tags"}
	cmd.AddCommand(s.newTagListCmd(token), s.newTagCreateCmd(token))
	return cmd
}

func (s *Service) newTagListCmd(token string) *cobra.Command {
	var socialSet string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags (GET /v2/social-sets/{id}/tags)",
		Long: "Tags belong to one social set. The `id` here is what `draft list --tag`\n" +
			"filters on — the name is not accepted there. There is no rename or delete\n" +
			"verb in this tool.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/tags"), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	return cmd
}

func (s *Service) newTagCreateCmd(token string) *cobra.Command {
	var socialSet, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tag (POST /v2/social-sets/{id}/tags)",
		Long: "--name is the tag text; Typefully assigns the id that filtering needs.\n" +
			"Because tags are scoped to a social set, the same name in two sets is two\n" +
			"different ids. Creating a tag does not attach it to anything — that goes\n" +
			"through `draft update --data`.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, scopedPath(socialSet, "/tags"), nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&name, "name", "", "tag name; required")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
