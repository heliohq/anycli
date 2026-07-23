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
		Use:         "get",
		Short:       "Get a user by email or userId (GET /api/users/{email} | /api/users/byUserId/{userId})",
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
		Use:         "update",
		Short:       "Create or update a user profile (POST /api/users/update)",
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
		Use:         "delete",
		Short:       "Delete a user by email (DELETE /api/users/{email})",
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
		Use:         "fields",
		Short:       "List the project's user data fields (GET /api/users/getFields)",
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
