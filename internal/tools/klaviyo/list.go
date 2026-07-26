package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The list read Longs. They sit here because the generic builders in common.go
// are resource-agnostic and the manual-vs-computed contrast with `segment` is
// exactly what a caller needs at this group.
const (
	longListList = "A Klaviyo list is an explicitly managed audience, which is why only lists\n" +
		"have add-profiles and remove-profiles commands; a segment is computed from\n" +
		"conditions and has no membership writes. Cursor-paged."

	longListGet = "The list's own record — name, creation time, opt-in settings — and not its\n" +
		"members, which come from `list profiles <id>`."
)

// newListCmd builds the `list` group: list/get/create plus the membership
// operations (profiles/add-profiles/remove-profiles).
func (s *Service) newListCmd(token string) *cobra.Command {
	group := newGroupCmd("list", "Manage lists and their membership")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List lists (GET /lists)", longListList, "/lists", "list"),
		s.newResourceGetCmd(token, "get", "Get one list (GET /lists/{id})", longListGet, "/lists/", "list"),
		s.newListCreateCmd(token),
		s.newListProfilesCmd(token),
		s.newListRelationshipCmd(token, "add-profiles",
			"Add profiles to a list (POST /lists/{id}/relationships/profiles)", longListAddProfiles, http.MethodPost),
		s.newListRelationshipCmd(token, "remove-profiles",
			"Remove profiles from a list (DELETE /lists/{id}/relationships/profiles)", longListRemoveProfiles, http.MethodDelete),
	)
	return group
}

func (s *Service) newListCreateCmd(token string) *cobra.Command {
	var name, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a list (POST /lists) from --name or --data",
		Long: "--name is the only shorthand; anything richer, including opt-in settings,\n" +
			"needs --data with a full JSON:API list body. The new list starts empty —\n" +
			"populate it with `list add-profiles`. Lists cannot be renamed or deleted\n" +
			"through this tool.",
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
		Use:   "profiles <id>",
		Short: "List a list's member profiles (GET /lists/{id}/profiles)",
		Long: "The list's members, one cursor page at a time; the shared --filter and\n" +
			"--sort apply to the PROFILES rather than to the list. A large list is many\n" +
			"calls, so --fields is worth using to trim each row to the attributes\n" +
			"actually needed.",
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

// longListAddProfiles and longListRemoveProfiles are the two membership Longs.
// They sit next to the shared builder because it is the builder that fixes the
// repeatable --profile-id and the 204 receipt both describe, while the
// membership-is-not-consent warning has to be phrased per direction.
const (
	longListAddProfiles = "--profile-id is repeatable and takes profile IDs, not email addresses —\n" +
		"resolve them with `profile list --filter 'equals(email,…)'` first. Adding\n" +
		"somebody to a list is a MEMBERSHIP change and not a consent change: it\n" +
		"subscribes nobody to marketing, which is `profile subscribe`. The endpoint\n" +
		"answers 204, so a local `{\"status\":\"ok\"}` receipt is printed instead."

	longListRemoveProfiles = "--profile-id is repeatable and takes profile ids. Removing somebody from a\n" +
		"list neither unsubscribes nor suppresses them: their consent is untouched\n" +
		"and any other list or segment still reaches them — use `profile\n" +
		"unsubscribe` or `profile suppress` to actually stop mail. The endpoint\n" +
		"answers 204, so a local `{\"status\":\"ok\"}` receipt is printed instead."
)

// newListRelationshipCmd builds add-profiles/remove-profiles: a to-many profile
// relationship mutation on a list. Profile ids come from repeatable
// --profile-id, or a raw --data body overrides.
func (s *Service) newListRelationshipCmd(token, use, short, long, method string) *cobra.Command {
	var profileIDs []string
	var data string
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
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
