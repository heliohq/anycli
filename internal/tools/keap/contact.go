package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newContactCmd(token string) *cobra.Command {
	cmd := newGroupCmd("contact", "Contacts (list, get, create, update, delete)")
	cmd.AddCommand(
		s.newContactListCmd(token),
		s.newContactGetCmd(token),
		s.newContactCreateCmd(token),
		s.newContactUpdateCmd(token),
		s.newContactDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newContactListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (GET /v2/contacts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/contacts", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newContactGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <contact-id>",
		Short: "Get a contact (GET /v2/contacts/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/contacts/"+url.PathEscape(args[0]), fieldsQuery(fields), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated fields to include")
	return cmd
}

// contactBodyFlags holds the convenience field flags shared by create/update.
type contactBodyFlags struct {
	email, givenName, familyName, phone string
	jobTitle, ownerID, contactType      string
	jsonBody                            string
}

func registerContactBodyFlags(cmd *cobra.Command) *contactBodyFlags {
	f := &contactBodyFlags{}
	cmd.Flags().StringVar(&f.email, "email", "", "primary email address")
	cmd.Flags().StringVar(&f.givenName, "given-name", "", "first name")
	cmd.Flags().StringVar(&f.familyName, "family-name", "", "last name")
	cmd.Flags().StringVar(&f.phone, "phone", "", "primary phone number")
	cmd.Flags().StringVar(&f.jobTitle, "job-title", "", "job title")
	cmd.Flags().StringVar(&f.ownerID, "owner-id", "", "owning user id")
	cmd.Flags().StringVar(&f.contactType, "contact-type", "", "contact type")
	cmd.Flags().StringVar(&f.jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload (custom_fields, etc.)")
	return f
}

// build assembles the v2 contact body from the convenience flags, then overlays
// --json-body (json-body keys win).
func (f *contactBodyFlags) build() (map[string]any, error) {
	body := map[string]any{}
	if f.givenName != "" {
		body["given_name"] = f.givenName
	}
	if f.familyName != "" {
		body["family_name"] = f.familyName
	}
	if f.jobTitle != "" {
		body["job_title"] = f.jobTitle
	}
	if f.ownerID != "" {
		body["owner_id"] = f.ownerID
	}
	if f.contactType != "" {
		body["contact_type"] = f.contactType
	}
	if f.email != "" {
		body["email_addresses"] = []map[string]any{{"email": f.email, "field": "EMAIL1"}}
	}
	if f.phone != "" {
		body["phone_numbers"] = []map[string]any{{"number": f.phone, "field": "PHONE1"}}
	}
	if err := applyJSONBody(body, f.jsonBody); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Service) newContactCreateCmd(token string) *cobra.Command {
	var f *contactBodyFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /v2/contacts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/contacts", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerContactBodyFlags(cmd)
	return cmd
}

func (s *Service) newContactUpdateCmd(token string) *cobra.Command {
	var f *contactBodyFlags
	cmd := &cobra.Command{
		Use:   "update <contact-id>",
		Short: "Update a contact (PATCH /v2/contacts/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/v2/contacts/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerContactBodyFlags(cmd)
	return cmd
}

func (s *Service) newContactDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <contact-id>",
		Short: "Delete a contact (DELETE /v2/contacts/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/v2/contacts/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
