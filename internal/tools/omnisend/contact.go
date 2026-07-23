package omnisend

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCmd builds the `contact` resource group: the audience the teammate
// looks up, adds, and tags.
func (s *Service) newContactCmd(token string) *cobra.Command {
	cmd := newGroupCmd("contact", "Contacts (list, get, create, update)")
	cmd.AddCommand(
		s.newContactListCmd(token),
		s.newContactGetCmd(token),
		s.newContactCreateCmd(token),
		s.newContactUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newContactListCmd(token string) *cobra.Command {
	var email string
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List contacts (GET /contacts)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyListQuery(q, limit, after)
			if email != "" {
				q.Set("email", email)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by contact email")
	registerListFlags(cmd, &limit, &after)
	return cmd
}

func (s *Service) newContactGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a contact by id (GET /contacts/{id})",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newContactCreateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a contact (POST /contacts). --data is the raw Omnisend contact JSON body.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw contact JSON body (Omnisend contact schema)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func (s *Service) newContactUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a contact (PATCH /contacts/{id}). --data is the raw partial JSON body.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/contacts/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&data, "data", "", "raw partial contact JSON body")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}
