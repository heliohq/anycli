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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
