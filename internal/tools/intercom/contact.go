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
		Long: "A whole-workspace walk, cursor-paginated at Intercom's default of 50 per\n" +
			"page and a ceiling of 150. Finding one person is `contact search --email`;\n" +
			"paging this to look someone up is the expensive way to get the same\n" +
			"answer.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "The convenience filters are --email, which is an EXACT equality match and\n" +
			"will not find a substring or a second address on the same person, and\n" +
			"--updated-since, compiled to `updated_at >` a Unix timestamp in seconds.\n" +
			"Anything else — name, external_id, custom attributes, role — needs a raw\n" +
			"--query object, which cannot be combined with the convenience flags. A\n" +
			"call with neither filter is rejected.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes Intercom's own contact id. An email address or your system's\n" +
			"external_id will not work here — resolve those through `contact search`\n" +
			"first, by --email for the former and a raw --query on `external_id` for\n" +
			"the latter.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--role picks user or lead, defaulting to user; a lead is an unidentified\n" +
			"visitor and converting one later is not exposed here. Intercom does not\n" +
			"deduplicate on email at this endpoint, so calling it twice for the same\n" +
			"address leaves two contacts behind — search first. Custom attributes and\n" +
			"any field without a flag go through --body-json, which is merged over the\n" +
			"scalar flags and wins on conflict.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Only the flags actually passed are sent, so unmentioned fields keep their\n" +
			"values. --role is the exception worth knowing: its `user` default is\n" +
			"dropped unless explicitly set, which is what stops a routine name change\n" +
			"from silently converting a lead into a user. --body-json merges over the\n" +
			"scalar flags for custom attributes.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Teammate-only, and attached to the PERSON rather than to a conversation,\n" +
			"so it stays visible across every future conversation with them — the right\n" +
			"place for standing context like account tier or escalation history.\n" +
			"--admin-id is genuinely optional here and is not resolved from /me, so\n" +
			"this write costs a single request.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "--tag-id comes from `tag list` and must be an id, not a name. There is no\n" +
			"matching untag for contacts — `conversation untag` covers conversations\n" +
			"only, so removing a tag from a person is a UI action.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
