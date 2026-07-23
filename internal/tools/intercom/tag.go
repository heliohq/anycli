package intercom

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTagCmd builds the tag resource group: list the workspace tags and
// create/update a tag. Intercom creates and updates tags through the same
// POST /tags endpoint (include id to update an existing tag).
func (s *Service) newTagCmd(token string) *cobra.Command {
	cmd := newGroupCmd("tag", "Tags: list, create")
	cmd.AddCommand(
		s.newTagListCmd(token),
		s.newTagCreateCmd(token),
	)
	return cmd
}

func (s *Service) newTagListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags (GET /tags)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/tags", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newTagCreateCmd(token string) *cobra.Command {
	var name, id string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create or update a tag (POST /tags)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"name": name}
			if id != "" {
				payload["id"] = id
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tags", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tag name")
	cmd.Flags().StringVar(&id, "id", "", "existing tag id (set to update rather than create)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
