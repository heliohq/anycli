package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCmd builds the contact resource group: the people CRM behind the
// inbox (look up who a customer is, create/update them, attach notes, tag).
func (s *Service) newContactCmd(token string) *cobra.Command {
	cmd := newGroupCmd("contact", "Contacts (people): look up, create, update, note, tag")
	cmd.AddCommand(
		s.newContactListCmd(token),
		s.newContactSearchCmd(token),
		s.newContactGetCmd(token),
		s.newContactCreateCmd(token),
		s.newContactUpdateCmd(token),
		s.newContactNoteCmd(token),
		s.newContactTagCmd(token),
	)
	return cmd
}

func (s *Service) newContactListCmd(token string) *cobra.Command {
	var perPage int
	var startingAfter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (GET /contacts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage > 0 {
				q.Set("per_page", intToString(perPage))
			}
			if startingAfter != "" {
				q.Set("starting_after", startingAfter)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page (Intercom default 50, max 150)")
	cmd.Flags().StringVar(&startingAfter, "starting-after", "", "pagination cursor from pages.next.starting_after")
	return cmd
}

func (s *Service) newContactSearchCmd(token string) *cobra.Command {
	var sf searchFlags
	var email, updatedSince string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search contacts (POST /contacts/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var filters []map[string]any
			if email != "" {
				filters = append(filters, filterEq("email", email))
			}
			if updatedSince != "" {
				filters = append(filters, filterGT("updated_at", updatedSince))
			}
			body, err := buildSearchBody(sf, filters)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerSearchFlags(cmd, &sf)
	cmd.Flags().StringVar(&email, "email", "", "convenience filter: exact email match")
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "convenience filter: updated_at > this Unix timestamp")
	return cmd
}

func (s *Service) newContactGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one contact (GET /contacts/{id})",
		Args:  cobra.NoArgs,
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
	var role, email, externalID, name, phone, bodyJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /contacts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := contactBody(role, email, externalID, name, phone, bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&role, "role", "user", "contact role: user|lead")
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "your system's external_id for the contact")
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&phone, "phone", "", "contact phone")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw contact JSON (merged; overrides the scalar flags)")
	return cmd
}

func (s *Service) newContactUpdateCmd(token string) *cobra.Command {
	var id, role, email, externalID, name, phone, bodyJSON string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a contact (PUT /contacts/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := contactBody(role, email, externalID, name, phone, bodyJSON)
			if err != nil {
				return err
			}
			// role is only meaningful on create; drop the default on update
			// unless the caller explicitly set it.
			if !cmd.Flags().Changed("role") {
				delete(payload, "role")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/contacts/"+url.PathEscape(id), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&role, "role", "user", "contact role: user|lead")
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "your system's external_id for the contact")
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&phone, "phone", "", "contact phone")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw contact JSON (merged; overrides the scalar flags)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// contactBody assembles a create/update contact payload from scalar flags,
// then merges an optional raw JSON object over the top (raw wins).
func contactBody(role, email, externalID, name, phone, bodyJSON string) (map[string]any, error) {
	payload := map[string]any{}
	if role != "" {
		payload["role"] = role
	}
	if email != "" {
		payload["email"] = email
	}
	if externalID != "" {
		payload["external_id"] = externalID
	}
	if name != "" {
		payload["name"] = name
	}
	if phone != "" {
		payload["phone"] = phone
	}
	if err := mergeBodyJSON(payload, bodyJSON); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Service) newContactNoteCmd(token string) *cobra.Command {
	var id, body, adminID string
	cmd := &cobra.Command{
		Use:   "note",
		Short: "Attach a note to a contact (POST /contacts/{id}/notes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"body": body}
			if adminID != "" {
				payload["admin_id"] = adminID
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts/"+url.PathEscape(id)+"/notes", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&body, "body", "", "note body")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "authoring admin id (optional)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newContactTagCmd(token string) *cobra.Command {
	var id, tagID string
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Add a tag to a contact (POST /contacts/{id}/tags)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"id": tagID}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts/"+url.PathEscape(id)+"/tags", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&tagID, "tag-id", "", "tag id to add")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("tag-id")
	return cmd
}
