package fullstory

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newUserCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "User profiles and properties"}
	cmd.AddCommand(
		s.newUserGetCmd(key),
		s.newUserListCmd(key),
		s.newUserUpsertCmd(key),
	)
	return cmd
}

// newUserGetCmd wraps GET /v2/users/{id} — resolve a single user by their
// FullStory-assigned id (from a session/user list result). --include-schema
// asks FullStory to include the property schema in the response.
func (s *Service) newUserGetCmd(key string) *cobra.Command {
	var id string
	var includeSchema bool
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a user by FullStory id (GET /v2/users/{id})",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "user get requires --id (the FullStory user id)"}
			}
			q := url.Values{}
			if includeSchema {
				q.Set("include_schema", "true")
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v2/users/"+url.PathEscape(id), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "FullStory-assigned user id")
	cmd.Flags().BoolVar(&includeSchema, "include-schema", false, "include the property schema in the response")
	return cmd
}

// newUserListCmd wraps GET /v2/users — find users matching a uid and/or email.
func (s *Service) newUserListCmd(key string) *cobra.Command {
	var uid, email string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Find users by uid and/or email (GET /v2/users)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if uid != "" {
				q.Set("uid", uid)
			}
			if email != "" {
				q.Set("email", email)
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v2/users", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "application-specific user id")
	cmd.Flags().StringVar(&email, "email", "", "user email")
	return cmd
}

// newUserUpsertCmd wraps POST /v2/users — create or update (by uid) a single
// user's server-side properties. FullStory upserts on uid: an existing uid is
// updated, a new one is created.
func (s *Service) newUserUpsertCmd(key string) *cobra.Command {
	var uid, displayName, email string
	var props []string
	cmd := &cobra.Command{
		Use:         "upsert",
		Short:       "Create or update a user by uid (POST /v2/users)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uid == "" {
				return &usageError{msg: "user upsert requires --uid"}
			}
			properties, perr := parseProps(props)
			if perr != nil {
				return perr
			}
			body := map[string]any{"uid": uid}
			if displayName != "" {
				body["display_name"] = displayName
			}
			if email != "" {
				body["email"] = email
			}
			if properties != nil {
				body["properties"] = properties
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/v2/users", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "application-specific user id (required)")
	cmd.Flags().StringVar(&displayName, "display-name", "", "user display name")
	cmd.Flags().StringVar(&email, "email", "", "user email")
	cmd.Flags().StringArrayVar(&props, "prop", nil, "custom property key=value (repeatable)")
	return cmd
}
