package brevo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCreateCmd builds `brevo contact create` — POST /contacts. With
// --update-enabled it upserts (Brevo force-merges on a shared identifier unless
// updateEnabled is set).
func (s *Service) newContactCreateCmd(apiKey string) *cobra.Command {
	var (
		email, extID, attributesJSON string
		listIDs                      []int
		updateEnabled                bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create or upsert a contact (POST /contacts)",
		Long: "Without `--update-enabled` this FAILS on a contact that already exists\n" +
			"rather than merging, so the safe form for import-style work is\n" +
			"`--update-enabled`. Identify the contact with `--email` or `--ext-id`.\n" +
			"`--attributes-json` is a raw JSON object whose keys are Brevo's uppercase\n" +
			"attribute names (`FIRSTNAME`, `SMS`), and `--list-ids` is repeatable and\n" +
			"takes integer list ids from `list ls`, subscribing the contact as it is\n" +
			"created.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if email != "" {
				body["email"] = email
			}
			if extID != "" {
				body["ext_id"] = extID
			}
			if attributesJSON != "" {
				attrs, err := decodeJSONObjectFlag("attributes-json", attributesJSON)
				if err != nil {
					return err
				}
				body["attributes"] = attrs
			}
			if len(listIDs) > 0 {
				body["listIds"] = listIDs
			}
			if cmd.Flags().Changed("update-enabled") {
				body["updateEnabled"] = updateEnabled
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodPost, "/contacts", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email (required unless --ext-id)")
	cmd.Flags().StringVar(&extID, "ext-id", "", "external id (ext_id)")
	cmd.Flags().StringVar(&attributesJSON, "attributes-json", "", "contact attributes as a raw JSON object (attribute names uppercase)")
	cmd.Flags().IntSliceVar(&listIDs, "list-ids", nil, "list id to add the contact to (repeatable, integer)")
	cmd.Flags().BoolVar(&updateEnabled, "update-enabled", false, "update the existing contact if it already exists (upsert)")
	return cmd
}

// newContactUpdateCmd builds `brevo contact update` — PUT /contacts/{identifier}.
func (s *Service) newContactUpdateCmd(apiKey string) *cobra.Command {
	var (
		id, identifierType, extID, attributesJSON string
		listIDs, unlinkListIDs                    []int
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a contact (PUT /contacts/{identifier})",
		Long: "`--id` is required and accepts an email, a contact id or an ext_id; add\n" +
			"`--identifier-type email_id|contact_id|ext_id` when the value could be\n" +
			"read either way. Only the attributes present in `--attributes-json` are\n" +
			"touched. This is also the membership verb: `--list-ids` subscribes and\n" +
			"`--unlink-list-ids` unsubscribes, both repeatable integers, and there is\n" +
			"no remove-from-list command elsewhere.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if extID != "" {
				body["ext_id"] = extID
			}
			if attributesJSON != "" {
				attrs, err := decodeJSONObjectFlag("attributes-json", attributesJSON)
				if err != nil {
					return err
				}
				body["attributes"] = attrs
			}
			if len(listIDs) > 0 {
				body["listIds"] = listIDs
			}
			if len(unlinkListIDs) > 0 {
				body["unlinkListIds"] = unlinkListIDs
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodPut, "/contacts/"+url.PathEscape(id), identifierTypeQuery(identifierType), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact identifier (email, contact id, or ext_id)")
	cmd.Flags().StringVar(&identifierType, "identifier-type", "", "how --id is interpreted: email_id|contact_id|ext_id")
	cmd.Flags().StringVar(&extID, "ext-id", "", "set the external id (ext_id)")
	cmd.Flags().StringVar(&attributesJSON, "attributes-json", "", "contact attributes as a raw JSON object")
	cmd.Flags().IntSliceVar(&listIDs, "list-ids", nil, "list id to add the contact to (repeatable)")
	cmd.Flags().IntSliceVar(&unlinkListIDs, "unlink-list-ids", nil, "list id to remove the contact from (repeatable)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newContactGetCmd builds `brevo contact get` — GET /contacts/{identifier}.
func (s *Service) newContactGetCmd(apiKey string) *cobra.Command {
	var id, identifierType string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact (GET /contacts/{identifier})",
		Long: "`--id` is required and takes an email address directly, so no lookup step\n" +
			"is needed to go from an address to a contact. Use `--identifier-type` when\n" +
			"the value is ambiguous — a numeric ext_id would otherwise be read as a\n" +
			"contact id. The response carries the contact's list memberships and\n" +
			"attributes.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/contacts/"+url.PathEscape(id), identifierTypeQuery(identifierType), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact identifier (email, contact id, or ext_id)")
	cmd.Flags().StringVar(&identifierType, "identifier-type", "", "how --id is interpreted: email_id|contact_id|ext_id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newContactListCmd builds `brevo contact list` — GET /contacts.
func (s *Service) newContactListCmd(apiKey string) *cobra.Command {
	var (
		limit, offset       int
		sort, modifiedSince string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List / search contacts (GET /contacts)",
		Long: "Walks the whole contact database — there is no search-by-attribute here,\n" +
			"so going from an email address to a contact is `contact get --id`, not a\n" +
			"scan. `--limit` defaults to 50 and tops out at 1000, and `--offset` pages.\n" +
			"`--modified-since` takes an ISO-8601 timestamp and is the cheap way to\n" +
			"pick up changes since a previous sweep; `--sort` is asc or desc by\n" +
			"creation.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("limit", itoa(limit))
			q.Set("offset", itoa(offset))
			if sort != "" {
				q.Set("sort", sort)
			}
			if modifiedSince != "" {
				q.Set("modifiedSince", modifiedSince)
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "page size (max 1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order by creation: asc|desc")
	cmd.Flags().StringVar(&modifiedSince, "modified-since", "", "only contacts modified since this ISO-8601 timestamp")
	return cmd
}

// newContactDeleteCmd builds `brevo contact delete` — DELETE /contacts/{identifier}.
func (s *Service) newContactDeleteCmd(apiKey string) *cobra.Command {
	var id, identifierType string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a contact (DELETE /contacts/{identifier})",
		Long: "`--id` is required, with the same email / contact id / ext_id handling and\n" +
			"`--identifier-type` as the other contact verbs. This removes the contact\n" +
			"and its history outright and cannot be undone. To stop mailing someone\n" +
			"without erasing them, drop them from the list instead with\n" +
			"`contact update --unlink-list-ids`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), apiKey, http.MethodDelete, "/contacts/"+url.PathEscape(id), identifierTypeQuery(identifierType), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact identifier (email, contact id, or ext_id)")
	cmd.Flags().StringVar(&identifierType, "identifier-type", "", "how --id is interpreted: email_id|contact_id|ext_id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// identifierTypeQuery returns the Brevo identifierType query set, or nil when no
// type is requested (Brevo then treats --id as an email or contact id).
func identifierTypeQuery(identifierType string) url.Values {
	if identifierType == "" {
		return nil
	}
	q := url.Values{}
	q.Set("identifierType", identifierType)
	return q
}
