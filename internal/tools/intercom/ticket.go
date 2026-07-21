package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTicketCmd builds the ticket resource group: structured support tickets
// created from a conversation or fresh, searched, updated, and replied to.
func (s *Service) newTicketCmd(token string) *cobra.Command {
	cmd := newGroupCmd("ticket", "Tickets: create, search, get, update, reply")
	cmd.AddCommand(
		s.newTicketCreateCmd(token),
		s.newTicketSearchCmd(token),
		s.newTicketGetCmd(token),
		s.newTicketUpdateCmd(token),
		s.newTicketReplyCmd(token),
		s.newTicketTypeListCmd(token),
	)
	return cmd
}

func (s *Service) newTicketCreateCmd(token string) *cobra.Command {
	var ticketTypeID, attributesJSON, bodyJSON string
	var contactIDs []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket (POST /tickets)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"ticket_type_id": ticketTypeID}
			if len(contactIDs) > 0 {
				contacts := make([]map[string]any, 0, len(contactIDs))
				for _, cid := range contactIDs {
					contacts = append(contacts, map[string]any{"id": cid})
				}
				payload["contacts"] = contacts
			}
			if attributesJSON != "" {
				v, err := decodeJSONFlag("attributes-json", attributesJSON)
				if err != nil {
					return err
				}
				payload["ticket_attributes"] = v
			}
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tickets", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&ticketTypeID, "ticket-type-id", "", "ticket type id (see `ticket type-list`)")
	cmd.Flags().StringArrayVar(&contactIDs, "contact-id", nil, "contact id to attach (repeatable)")
	cmd.Flags().StringVar(&attributesJSON, "attributes-json", "", "ticket_attributes as a JSON object")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw ticket JSON (merged; overrides the scalar flags)")
	_ = cmd.MarkFlagRequired("ticket-type-id")
	return cmd
}

func (s *Service) newTicketSearchCmd(token string) *cobra.Command {
	var sf searchFlags
	var updatedSince string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search tickets (POST /tickets/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var filters []map[string]any
			if updatedSince != "" {
				filters = append(filters, filterGT("updated_at", updatedSince))
			}
			body, err := buildSearchBody(sf, filters)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tickets/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerSearchFlags(cmd, &sf)
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "convenience filter: updated_at > this Unix timestamp")
	return cmd
}

func (s *Service) newTicketGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one ticket (GET /tickets/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/tickets/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTicketUpdateCmd(token string) *cobra.Command {
	var id, state, assigneeID, adminID, attributesJSON, bodyJSON string
	var open bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a ticket state/assignment/attributes (PUT /tickets/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{}
			if state != "" {
				payload["state"] = state
			}
			if cmd.Flags().Changed("open") {
				payload["open"] = open
			}
			if assigneeID != "" || adminID != "" {
				assignment := map[string]any{}
				if adminID != "" {
					assignment["admin_id"] = adminID
				}
				if assigneeID != "" {
					assignment["assignee_id"] = assigneeID
				}
				payload["assignment"] = assignment
			}
			if attributesJSON != "" {
				v, err := decodeJSONFlag("attributes-json", attributesJSON)
				if err != nil {
					return err
				}
				payload["ticket_attributes"] = v
			}
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/tickets/"+url.PathEscape(id), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringVar(&state, "state", "", "ticket state (e.g. submitted|in_progress|waiting_on_customer|resolved)")
	cmd.Flags().BoolVar(&open, "open", false, "open/close flag")
	cmd.Flags().StringVar(&assigneeID, "assignee-id", "", "target admin or team id")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id")
	cmd.Flags().StringVar(&attributesJSON, "attributes-json", "", "ticket_attributes as a JSON object")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw ticket JSON (merged; overrides the scalar flags)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTicketReplyCmd(token string) *cobra.Command {
	var id, body, adminID, messageType string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to a ticket as an admin (POST /tickets/{id}/reply)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			admin, err := s.resolveAdminID(cmd.Context(), token, adminID)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"message_type": messageType,
				"type":         "admin",
				"admin_id":     admin,
				"body":         body,
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/tickets/"+url.PathEscape(id)+"/reply", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "ticket id")
	cmd.Flags().StringVar(&body, "body", "", "reply body (HTML allowed)")
	cmd.Flags().StringVar(&messageType, "message-type", "comment", "comment (customer-visible) or note (internal)")
	cmd.Flags().StringVar(&adminID, "admin-id", "", "acting admin id (defaults to the /me admin)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newTicketTypeListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "type-list",
		Short: "List ticket types (GET /ticket_types)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/ticket_types", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

// mergeBodyJSON merges an optional raw JSON object into an existing payload
// (raw wins). An empty raw string is a no-op; a non-object raw is a usage error.
func mergeBodyJSON(payload map[string]any, bodyJSON string) error {
	if bodyJSON == "" {
		return nil
	}
	v, err := decodeJSONFlag("body-json", bodyJSON)
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return &usageError{msg: "intercom: --body-json must be a JSON object"}
	}
	for k, val := range m {
		payload[k] = val
	}
	return nil
}
