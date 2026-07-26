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
		Long: "The workspace's tag vocabulary. Every tagging verb takes an id from here,\n" +
			"never a name: `conversation tag`, `conversation untag` and `contact tag`\n" +
			"all reject a tag name. Unpaginated.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "One endpoint does both: with --id the named tag is renamed, and the new\n" +
			"name shows up everywhere that tag is already applied; without --id a new\n" +
			"tag is created. Removing a tag is not possible from here at all — that is\n" +
			"a UI action, and it strips the tag from every conversation and contact\n" +
			"carrying it.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
