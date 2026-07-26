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
		Long: "Contacts are the account's central record: tags, notes, tasks,\n" +
			"opportunities and `email send` all address a contact by the numeric `id`\n" +
			"returned here. Resolve an address to an id with\n" +
			"`--filter email==jo@x.com` rather than paging the whole list.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the numeric contact id, not an email address — resolve an address\n" +
			"with `contact list --filter email==jo@x.com` first. `--fields` trims the\n" +
			"response to a comma-separated projection of the v2 contact schema\n" +
			"(`given_name`, `email_addresses`, `custom_fields`, ...).",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "`--email` and `--phone` are expanded into Keap's `email_addresses` and\n" +
			"`phone_numbers` arrays in the `EMAIL1` / `PHONE1` slots, so each flag sets\n" +
			"only the primary entry. Further slots, postal addresses and `custom_fields`\n" +
			"go through `--json-body`, whose keys overlay and win over the flags. At\n" +
			"least one field must be supplied or the call is rejected before it is sent.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Only the fields supplied are touched. `--email` and `--phone` send a\n" +
			"complete `email_addresses` / `phone_numbers` array holding just the EMAIL1 /\n" +
			"PHONE1 entry, so a contact with several stored addresses needs the full\n" +
			"array passed through `--json-body` instead. At least one field is required.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Irreversible — there is no restore verb, and notes live under the contact\n" +
			"path (`/v2/contacts/{id}/notes`) so they are not separately recoverable.\n" +
			"Judge the outcome from the exit code rather than stdout.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
