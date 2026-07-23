package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListCmd builds the `list` group: list/get/create plus the membership
// operations (profiles/add-profiles/remove-profiles).
func (s *Service) newListCmd(token string) *cobra.Command {
	group := newGroupCmd("list", "Manage lists and their membership")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List lists (GET /lists)", "/lists", "list"),
		s.newResourceGetCmd(token, "get", "Get one list (GET /lists/{id})", "/lists/", "list"),
		s.newListCreateCmd(token),
		s.newListProfilesCmd(token),
		s.newListRelationshipCmd(token, "add-profiles",
			"Add profiles to a list (POST /lists/{id}/relationships/profiles)", http.MethodPost),
		s.newListRelationshipCmd(token, "remove-profiles",
			"Remove profiles from a list (DELETE /lists/{id}/relationships/profiles)", http.MethodDelete),
	)
	return group
}

func (s *Service) newListCreateCmd(token string) *cobra.Command {
	var name, data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a list (POST /lists) from --name or --data",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var payload any
			if data != "" {
				var err error
				if payload, err = parseDataFlag(data); err != nil {
					return err
				}
			} else if name != "" {
				payload = resourceBody("list", "", map[string]any{"name": name}, nil)
			} else {
				return &usageError{msg: "provide --name, or --data"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/lists", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "list name")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides --name)")
	return cmd
}

func (s *Service) newListProfilesCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:         "profiles <id>",
		Short:       "List a list's member profiles (GET /lists/{id}/profiles)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := f.query("profile")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/lists/"+url.PathEscape(args[0])+"/profiles", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}

// newListRelationshipCmd builds add-profiles/remove-profiles: a to-many profile
// relationship mutation on a list. Profile ids come from repeatable
// --profile-id, or a raw --data body overrides.
func (s *Service) newListRelationshipCmd(token, use, short, method string) *cobra.Command {
	var profileIDs []string
	var data string
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			var payload any
			if data != "" {
				var err error
				if payload, err = parseDataFlag(data); err != nil {
					return err
				}
			} else if len(profileIDs) > 0 {
				payload = relationshipBody("profile", profileIDs)
			} else {
				return &usageError{msg: "provide at least one --profile-id, or --data"}
			}
			body, err := s.call(cmd.Context(), token, method, "/lists/"+url.PathEscape(args[0])+"/relationships/profiles", nil, payload)
			if err != nil {
				return err
			}
			// Relationship mutations return 204 No Content; surface a receipt so
			// stdout is never silently empty on success.
			if len(body) == 0 {
				return s.emit([]byte(`{"status":"ok"}`))
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&profileIDs, "profile-id", nil, "profile id to add/remove (repeatable)")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API relationship body (overrides --profile-id)")
	return cmd
}
