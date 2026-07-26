package iterable

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserCmd groups the user (contact) profile verbs.
func (s *Service) newUserCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("user", "Manage user (contact) profiles")
	cmd.AddCommand(
		s.newUserGetCmd(cred),
		s.newUserUpdateCmd(cred),
		s.newUserDeleteCmd(cred),
		s.newUserFieldsCmd(cred),
	)
	return cmd
}

func (s *Service) newUserGetCmd(cred credential) *cobra.Command {
	var email, userID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a user by email or userId (GET /api/users/{email} | /api/users/byUserId/{userId})",
		Long: "--email and --user-id are mutually exclusive and exactly one is required;\n" +
			"they resolve to different endpoints, so a userId passed as --email is a\n" +
			"lookup miss rather than a usage error. Every custom attribute on the\n" +
			"profile sits under `dataFields`, and `user fields` names the ones this\n" +
			"project defines.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := userLookupPath(email, userID)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "user email (mutually exclusive with --user-id)")
	cmd.Flags().StringVar(&userID, "user-id", "", "user id (mutually exclusive with --email)")
	return cmd
}

func (s *Service) newUserUpdateCmd(cred credential) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Create or update a user profile (POST /api/users/update)",
		Long: "This UPSERTS: it creates the profile when the email or userId is unknown\n" +
			"and updates it when it is known, which is why there is no separate create\n" +
			"command. --body is required and must carry at least `email` or `userId`.\n" +
			"Custom attributes go under `dataFields` — check `user fields` for the\n" +
			"names and types this project already has.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("body", body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodPost, "/api/users/update", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `JSON body, e.g. {"email":"a@b.com","dataFields":{"firstName":"Ada"}} (required)`)
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newUserDeleteCmd(cred credential) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a user by email (DELETE /api/users/{email})",
		Long: "Takes --email only; there is no delete-by-userId variant. It removes the\n" +
			"profile from the project and there is no undo — a later `user update` with\n" +
			"the same address creates a NEW profile carrying none of the old history or\n" +
			"event data.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return &usageError{msg: "iterable: --email is required"}
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodDelete, "/api/users/"+url.PathEscape(email), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "user email to delete (required)")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newUserFieldsCmd(cred credential) *cobra.Command {
	return &cobra.Command{
		Use:   "fields",
		Short: "List the project's user data fields (GET /api/users/getFields)",
		Long: "Returns the project's user field schema: every `dataFields` key and the\n" +
			"type Iterable has assigned it. Read it before `user update`, because\n" +
			"Iterable creates an unknown field on first write and fixes its type from\n" +
			"that first value — a later write of a different type to the same name is\n" +
			"rejected.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/users/getFields", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// userLookupPath resolves the by-email vs by-userId GET path, requiring exactly
// one selector.
func userLookupPath(email, userID string) (string, error) {
	switch {
	case email != "" && userID != "":
		return "", &usageError{msg: "iterable: pass exactly one of --email or --user-id, not both"}
	case email != "":
		return "/api/users/" + url.PathEscape(email), nil
	case userID != "":
		return "/api/users/byUserId/" + url.PathEscape(userID), nil
	default:
		return "", &usageError{msg: "iterable: one of --email or --user-id is required"}
	}
}
